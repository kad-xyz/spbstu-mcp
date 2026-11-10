// Package openedu reads openedu.ru and its Open edX LMS (courses.openedu.ru), signing in
// through the SPbPU identity provider of the openedu.ru SSO, or with an openedu.ru email and
// password: openedu.ru keeps these as two separate accounts.
package openedu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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

// Keys of the session store.
const (
	cookiesKey = "openedu.cookies"
	loginKey   = "openedu.login"
)

var (
	brokerRe = regexp.MustCompile(`/realms/openedu/broker/spbstu/login\?[^"\\]+`)
	// The sign-in page is a Keycloakify theme that renders the form from a kcContext object;
	// the plain Keycloak theme has the form in the HTML.
	loginActionRe = regexp.MustCompile(`"loginAction":\s*"([^"]+)"|<form[^>]*id="kc-form-login"[^>]*action="([^"]+)"`)
	// Other pages, e.g. a required password change, carry a loginAction too.
	loginPageRe = regexp.MustCompile(`"pageId":\s*"login"|id="kc-form-login"`)
	kcSummaryRe = regexp.MustCompile(`"summary":\s*"((?:[^"\\]|\\.)*)"`)
	kcErrorRe   = regexp.MustCompile(`id="input-error[^"]*"[^>]*>\s*([^<]+?)\s*<`)
)

// loadPassword reads the openedu.ru password; tests replace it.
var loadPassword = keychain.Load

// ErrRejected is returned when openedu.ru turns down the email or password.
var ErrRejected = errors.New("openedu.ru rejected the email or password")

// PasswordService is the credential store service that holds openedu.ru passwords next to
// the university logins of service, one item per openedu.ru email.
func PasswordService(service string) string { return service + "-openedu" }

// Client keeps openedu.ru and courses.openedu.ru sessions and signs in when one lapses.
type Client struct {
	KeychainService string
	// KeychainAccount picks the account of the service to sign in with; "" takes the first one.
	KeychainAccount string
	// State keeps the session cookies and the openedu.ru login between runs; nil keeps them
	// in memory only.
	State *state.Store

	mu     sync.Mutex
	client *http.Client
	email  string // the openedu.ru login when State is nil
}

func (c *Client) init() {
	if c.client == nil {
		c.client = &http.Client{Jar: state.NewJar(c.State, cookiesKey)}
	}
}

// login is the openedu.ru email the client signs in with, or "" for «Политех»; c.mu is held.
func (c *Client) login() string {
	if c.State == nil {
		return c.email
	}
	return c.State.Get(loginKey)
}

// setLogin switches to another way of signing in, which means another openedu.ru account,
// so its sessions replace the current ones; c.mu is held.
func (c *Client) setLogin(login, cookies string) error {
	if c.State == nil {
		c.email = login
	} else {
		if err := c.State.Set(loginKey, login); err != nil {
			return err
		}
		if err := c.State.Set(cookiesKey, cookies); err != nil {
			return err
		}
	}
	c.init()
	c.client.Jar = state.NewJar(c.State, cookiesKey)
	return nil
}

// SignInAs makes the client sign in with an openedu.ru email and its password from the
// credential store, or through «Политех» when email is "". It signs in once to check, or
// only checks the session when the way stays the same, and returns the openedu.ru username;
// when that fails, the previous way and its sessions stay.
func (c *Client) SignInAs(ctx context.Context, email string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	prevLogin, prevCookies := c.login(), ""
	if c.State != nil {
		prevCookies = c.State.Get(cookiesKey)
	}
	if email != prevLogin {
		if err := c.setLogin(email, ""); err != nil {
			return "", err
		}
	}
	var me struct {
		Username string `json:"username"`
	}
	body, err := c.lms(ctx, "/api/user/v1/me")
	if err == nil {
		err = json.Unmarshal(body, &me)
	}
	if err != nil && email != prevLogin {
		if rerr := c.setLogin(prevLogin, prevCookies); rerr != nil {
			return "", errors.Join(err, rerr)
		}
	}
	if err != nil {
		return "", err
	}
	return me.Username, nil
}

// accountHint explains "not enrolled" for a course the user sees on openedu.ru: the server
// may be signed in to the other openedu.ru account.
func accountHint(login string) string {
	if login == "" {
		return "The server signs in to openedu.ru through «Политех». If the user signs in to openedu.ru with email and " +
			"password, that is a separate openedu.ru account with its own courses: switch to it with openedu_login"
	}
	return "The server signs in to openedu.ru as " + login + ". If the course is in another openedu.ru account, " +
		"e.g. the one reached through «Политех», switch with openedu_login"
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
	return c.lms(ctx, path)
}

// lms is LMS with c.mu held.
func (c *Client) lms(ctx context.Context, path string) ([]byte, error) {
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
		case bytes.Contains(body, []byte("not enrolled")):
			return nil, fmt.Errorf("courses.openedu.ru%s: HTTP %d: %.200s. %s", path, status, body, accountHint(c.login()))
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
// the SPbPU identity provider with the keychain credentials, or with the openedu.ru email
// and password.
func (c *Client) signIn(ctx context.Context, start, host string) error {
	page, _, final, err := c.fetch(ctx, start)
	if err != nil {
		return err
	}
	if signedIn(final) {
		return nil
	}
	if email := c.login(); email != "" {
		return c.passwordSignIn(ctx, page, final, host, email)
	}
	broker := brokerRe.Find(page)
	if broker == nil {
		return fmt.Errorf("%s sign-in: SPbPU option not found at %s%s", host, final.Host, final.Path)
	}
	creds, err := keychain.Load(c.KeychainService, c.KeychainAccount)
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

// passwordSignIn fills in the email and password form of the openedu.ru SSO page.
func (c *Client) passwordSignIn(ctx context.Context, page []byte, at *url.URL, host, email string) error {
	action := loginAction(page)
	if action == "" {
		return fmt.Errorf("%s sign-in: email and password form not found at %s%s", host, at.Host, at.Path)
	}
	creds, err := loadPassword(PasswordService(c.KeychainService), email)
	if err != nil {
		return err
	}
	form := url.Values{
		"username":     {email},
		"password":     {creds.Password},
		"credentialId": {""},
		// A remembered SSO session lasts longer, so the server signs in less often.
		"rememberMe": {"on"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, action, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	end := resp.Request.URL
	if signedIn(end) {
		return nil
	}
	if loginPageRe.Match(body) {
		// The form came back: Keycloak shows it again with the reason.
		if msg := kcMessage(body); msg != "" {
			return fmt.Errorf("%w: %s", ErrRejected, msg)
		}
		return ErrRejected
	}
	return errors.New(host + " sign-in as " + email + " stopped at " + end.Host + end.Path + " (finish it once in a browser)")
}

// loginAction finds where the SSO page posts its email and password form.
func loginAction(page []byte) string {
	m := loginActionRe.FindSubmatch(page)
	switch {
	case m == nil:
		return ""
	case m[1] != nil:
		return strings.ReplaceAll(string(m[1]), `\/`, "/")
	default:
		return html.UnescapeString(string(m[2]))
	}
}

// kcMessage is the error Keycloak shows above its form, e.g. «Неправильное имя пользователя
// или пароль».
func kcMessage(page []byte) string {
	if m := kcSummaryRe.FindSubmatch(page); m != nil {
		s := strings.ReplaceAll(string(m[1]), `\/`, "/")
		if u, err := strconv.Unquote(`"` + s + `"`); err == nil {
			s = u
		}
		return strings.TrimSpace(html.UnescapeString(s))
	}
	if m := kcErrorRe.FindSubmatch(page); m != nil {
		return html.UnescapeString(string(m[1]))
	}
	return ""
}

// signedIn reports whether a sign-in ended back on openedu.ru or its LMS rather than on a
// login page: the LMS hands the browser on to the portal once it has its session.
func signedIn(u *url.URL) bool {
	return u.Host == "openedu.ru" || u.Host == "courses.openedu.ru"
}
