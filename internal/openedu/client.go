// Package openedu reads openedu.ru and its Open edX LMS (courses.openedu.ru), signing in
// through the SPbPU identity provider of the openedu.ru SSO.
package openedu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/kad-xyz/spbstu-mcp/internal/cas"
	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

const (
	PortalURL = "https://openedu.ru"
	LMSURL    = "https://courses.openedu.ru"
	ssoURL    = "https://sso.openedu.ru"

	portalLogin = PortalURL + "/auth/login/npoedsso/"
	lmsLogin    = LMSURL + "/auth/login/keycloak/?auth_entry=login&next=%2F"
)

var (
	brokerRe = regexp.MustCompile(`/realms/openedu/broker/spbstu/login\?[^"\\]+`)
)

// Client keeps openedu.ru and courses.openedu.ru sessions and signs in when one lapses.
type Client struct {
	KeychainService string
	// State keeps the session cookies between runs; nil keeps them in memory only.
	State *state.Store

	mu     sync.Mutex
	client *http.Client
}

func (c *Client) init() {
	if c.client == nil {
		c.client = &http.Client{Jar: state.NewJar(c.State, "openedu.cookies")}
	}
}

// Page returns the HTML of an openedu.ru page, signing in first when needed.
func (c *Client) Page(ctx context.Context, path string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	for attempt := 0; ; attempt++ {
		body, status, final, err := c.fetch(ctx, PortalURL+path)
		if err != nil {
			return nil, err
		}
		// A signed-in page links to the SSO logout.
		if final.Host == "openedu.ru" && status == http.StatusOK && strings.Contains(string(body), "/auth/sso-logout") {
			return body, nil
		}
		if attempt > 0 {
			return nil, fmt.Errorf("openedu.ru%s: not signed in after signing in (HTTP %d)", path, status)
		}
		if err := c.signIn(ctx, portalLogin, "openedu.ru"); err != nil {
			return nil, err
		}
	}
}

// API decodes a JSON response of the LMS. When the short-lived JWT cookie has expired it is
// refreshed from the LMS session, and only when that has lapsed too does it sign in again.
func (c *Client) API(ctx context.Context, path string, out any) error {
	body, err := c.LMS(ctx, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// LMS returns the body of an LMS page or API response, with the same session handling as API.
// A page that redirects away from the LMS (to the SSO login) counts as unauthorized.
func (c *Client) LMS(ctx context.Context, path string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	var signInErr error
	for attempt := 0; ; attempt++ {
		body, status, final, err := c.fetch(ctx, LMSURL+path)
		if err != nil {
			return nil, err
		}
		left := final.Host != "courses.openedu.ru"
		if status == http.StatusOK && !left {
			return body, nil
		}
		unauthorized := status == http.StatusUnauthorized || status == http.StatusForbidden || left
		switch {
		case unauthorized && attempt == 0:
			c.refreshJWT(ctx)
		case unauthorized && attempt == 1:
			// The sign-in can end in a redirect loop on /auth/complete/ although the session is
			// established, so the retry decides, not the error.
			signInErr = c.signIn(ctx, lmsLogin, "courses.openedu.ru")
		case signInErr != nil:
			return nil, signInErr
		default:
			return nil, fmt.Errorf("courses.openedu.ru%s: HTTP %d: %.200s", path, status, body)
		}
	}
}

// refreshJWT asks the LMS for new JWT cookies on the strength of its session cookie.
func (c *Client) refreshJWT(ctx context.Context) {
	lms, _ := url.Parse(LMSURL)
	csrf := ""
	for _, ck := range c.client.Jar.Cookies(lms) {
		if ck.Name == "csrftoken" {
			csrf = ck.Value
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, LMSURL+"/login_refresh", nil)
	if err != nil {
		return
	}
	req.Header.Set("X-CSRFToken", csrf)
	req.Header.Set("Referer", LMSURL+"/")
	if resp, err := c.client.Do(req); err == nil {
		resp.Body.Close()
	}
}

func (c *Client) fetch(ctx context.Context, u string) ([]byte, int, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, nil, err
	}
	req.Header.Set("Accept", "application/json, text/html")
	// Open edX accepts its JWT cookies only from callers that ask for it.
	req.Header.Set("USE-JWT-COOKIE", "true")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, resp.Request.URL, err
}

// signIn opens a site's SSO login; when the SSO session has lapsed too, it signs in through
// the SPbPU identity provider with the keychain credentials.
func (c *Client) signIn(ctx context.Context, start, host string) error {
	page, _, final, err := c.fetch(ctx, start)
	if err != nil {
		return err
	}
	if signedIn(final) {
		return nil
	}
	broker := brokerRe.Find(page)
	if broker == nil {
		return fmt.Errorf("%s sign-in: SPbPU option not found at %s%s", host, final.Host, final.Path)
	}
	creds, err := keychain.Load(c.KeychainService)
	if err != nil {
		return err
	}
	brokerURL := ssoURL + strings.ReplaceAll(html.UnescapeString(string(broker)), `&`, "&")
	end, err := cas.Login(ctx, c.client, brokerURL, creds.Login(), creds.Password)
	if err != nil {
		return err
	}
	if !signedIn(end) {
		return errors.New(host + " sign-in stopped at " + end.Host + end.Path + " (the account may need linking in a browser first)")
	}
	return nil
}

// signedIn reports whether a sign-in ended back on openedu.ru or its LMS rather than on a
// login page: the LMS hands the browser on to the portal once it has its session.
func signedIn(u *url.URL) bool {
	return u.Host == "openedu.ru" || u.Host == "courses.openedu.ru"
}
