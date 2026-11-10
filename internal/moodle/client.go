// Package moodle talks to SPbPU Moodle sites through the mobile web service API.
package moodle

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kad-xyz/spbstu-mcp/internal/cas"
	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

const DefaultSite = "dl.spbstu.ru"

type siteConfig struct {
	host        string
	clientID    string
	redirectURL string
}

// siteConfigs is both the SSRF allow-list and the per-installation OAuth configuration.
// The OAuth values come from each installation's public login page and are not interchangeable.
var siteConfigs = []siteConfig{
	{
		host:        DefaultSite,
		clientID:    "5cae4f19-00ac-4910-b731-8e44065ad93d",
		redirectURL: "https://dl.spbstu.ru/auth/spbstu/redirect.php?auth_service=spbstu_oauth2",
	},
	{
		host:        "dl-hum.spbstu.ru",
		clientID:    "ae6fe956-d324-4ad5-88d7-c53b32f94452",
		redirectURL: "https://dl-hum.spbstu.ru/auth/spbstu/redirect.php?auth_service=spbstu_oauth2",
	},
	{
		host:        "lms.spbstu.ru",
		clientID:    "94d5f926-5aae-4320-b796-3eb79b4f7cb8",
		redirectURL: "https://lms.spbstu.ru/auth/spbstu/redirect.php?auth_service=spbstu_oauth2",
	},
}

// Sites lists the known Moodle hosts in stable display order; the server uses the ones
// enabled with -moodle-sites.
var Sites = func() []string {
	sites := make([]string, 0, len(siteConfigs))
	for _, config := range siteConfigs {
		sites = append(sites, config.host)
	}
	return sites
}()

// Known reports whether site is one of Sites.
func Known(site string) bool {
	for _, s := range Sites {
		if s == site {
			return true
		}
	}
	return false
}

// ErrNoSession means the site has no saved token, so using it would sign in through the SSO.
var ErrNoSession = errors.New("no saved session yet")

// Client is a lazily authenticated Moodle web service client. It signs in through the SPbPU
// SSO with the keychain credentials on first use and again whenever the token stops working.
type Client struct {
	// Site is a host from Sites; empty means DefaultSite.
	Site            string
	KeychainService string
	// KeychainAccount picks the account of the service to sign in with; "" takes the first one.
	KeychainAccount string
	// State keeps the token between runs; nil keeps it in memory only.
	State *state.Store

	mu     sync.Mutex
	token  string
	userID int
	// refused is the error of a sign-in the SSO let through but the site itself turned
	// down. It is kept for the life of the process: trying again only sends the password
	// to the SSO once more, and frequent sign-ins get the login locked.
	refused error
}

// Ready says whether the client can be used without a new sign-in: nil when it holds or has
// saved a token, ErrNoSession when it has none, or the error of the site's refused sign-in.
func (c *Client) Ready() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refused != nil {
		return c.refused
	}
	if c.token != "" || c.savedToken() != "" {
		return nil
	}
	return ErrNoSession
}

// savedToken is the token kept in the state store, if any.
func (c *Client) savedToken() string {
	if c.State == nil {
		return ""
	}
	key, err := c.tokenKey()
	if err != nil {
		return ""
	}
	token := c.State.Get(key)
	// Keep existing installations from signing in again after the token key became
	// site-specific. New tokens are always written under the new key.
	if token == "" && (c.Site == "" || c.Site == DefaultSite) {
		token = c.State.Get("dl.token")
	}
	return token
}

func (c *Client) site() (string, error) {
	config, err := c.config()
	if err != nil {
		return "", err
	}
	return config.host, nil
}

func (c *Client) config() (siteConfig, error) {
	site := c.Site
	if site == "" {
		site = DefaultSite
	}
	for _, config := range siteConfigs {
		if site == config.host {
			return config, nil
		}
	}
	return siteConfig{}, fmt.Errorf("unsupported Moodle site %q", site)
}

func (c *Client) authorizeURL() (string, error) {
	config, err := c.config()
	if err != nil {
		return "", err
	}
	return "https://cas.spbstu.ru/oauth2.0/authorize?client_id=" + config.clientID +
		"&redirect_uri=" + url.QueryEscape(config.redirectURL) + "&response_type=code", nil
}

func (c *Client) baseURL() (string, error) {
	site, err := c.site()
	if err != nil {
		return "", err
	}
	return "https://" + site, nil
}

func (c *Client) tokenKey() (string, error) {
	site, err := c.site()
	if err != nil {
		return "", err
	}
	return "moodle." + site + ".token", nil
}

// Error is an exception returned by the Moodle web service.
type Error struct {
	Exception string `json:"exception"`
	ErrorCode string `json:"errorcode"`
	Message   string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("moodle: %s (%s)", e.Message, e.ErrorCode) }

func (e *Error) authFailure() bool {
	switch e.ErrorCode {
	case "invalidtoken", "accessexception", "servicerequireslogin":
		return true
	}
	return false
}

// UserID is the id of the signed-in user.
func (c *Client) UserID(ctx context.Context) (int, error) {
	if _, err := c.session(ctx); err != nil {
		return 0, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userID, nil
}

// Call runs a web service function and decodes its JSON result into out.
func (c *Client) Call(ctx context.Context, function string, params url.Values, out any) error {
	token, err := c.session(ctx)
	if err != nil {
		return err
	}
	err = c.call(ctx, token, function, params, out)
	var merr *Error
	if errors.As(err, &merr) && merr.authFailure() {
		c.forget()
		if token, err = c.session(ctx); err != nil {
			return err
		}
		err = c.call(ctx, token, function, params, out)
	}
	return err
}

func (c *Client) call(ctx context.Context, token, function string, params url.Values, out any) error {
	baseURL, err := c.baseURL()
	if err != nil {
		return err
	}
	form := url.Values{}
	for k, v := range params {
		form[k] = v
	}
	form.Set("wstoken", token)
	form.Set("wsfunction", function)
	form.Set("moodlewsrestformat", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/webservice/rest/server.php", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(body) > 0 && body[0] == '{' {
		var merr Error
		if json.Unmarshal(body, &merr) == nil && merr.Exception != "" {
			return &merr
		}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// Download saves a pluginfile URL from the API into dir and returns the file's path.
func (c *Client) Download(ctx context.Context, fileURL, dir string) (string, error) {
	site, err := c.site()
	if err != nil {
		return "", err
	}
	u, err := url.Parse(fileURL)
	if err != nil || u.Scheme != "https" || u.Host != site {
		return "", fmt.Errorf("not a %s HTTPS file URL: %s", site, fileURL)
	}
	token, err := c.session(ctx)
	if err != nil {
		return "", err
	}
	if !strings.Contains(u.Path, "/webservice/pluginfile.php") {
		u.Path = strings.Replace(u.Path, "/pluginfile.php", "/webservice/pluginfile.php", 1)
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		var merr Error
		if json.NewDecoder(resp.Body).Decode(&merr) == nil && merr.Exception != "" {
			return "", &merr
		}
		return "", errors.New("download failed: unexpected JSON response")
	}
	name, _ := url.PathUnescape(filepath.Base(u.Path))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return "", err
	}
	return path, f.Close()
}

// session returns a working token, signing in when there is none.
func (c *Client) session(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" {
		return c.token, nil
	}
	var info struct {
		UserID int `json:"userid"`
	}
	if c.refused != nil {
		return "", c.refused
	}
	tokenKey, err := c.tokenKey()
	if err != nil {
		return "", err
	}
	if token := c.savedToken(); token != "" {
		err := c.call(ctx, token, "core_webservice_get_site_info", nil, &info)
		if err == nil && info.UserID != 0 {
			c.token, c.userID = token, info.UserID
			return token, nil
		}
		// An unreachable site says nothing about the token; signing in would only send the
		// password to the SSO once more.
		var netErr *url.Error
		if errors.As(err, &netErr) {
			return "", err
		}
	}
	token, err := c.signIn(ctx)
	var refused *refusedError
	if errors.As(err, &refused) {
		c.refused = err
	}
	if err != nil {
		return "", err
	}
	if err := c.call(ctx, token, "core_webservice_get_site_info", nil, &info); err != nil {
		return "", err
	}
	c.token, c.userID = token, info.UserID
	if c.State != nil {
		_ = c.State.Set(tokenKey, token)
	}
	return token, nil
}

// forget drops a token the server no longer accepts.
func (c *Client) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = ""
	if c.State != nil {
		if tokenKey, err := c.tokenKey(); err == nil {
			_ = c.State.Set(tokenKey, "")
		}
	}
}

// signIn goes through the SSO and asks the mobile app launcher for a token, which it hands
// back as a moodlemobile:// redirect carrying base64("siteid:::token[:::privatetoken]").
func (c *Client) signIn(ctx context.Context) (string, error) {
	site, err := c.site()
	if err != nil {
		return "", err
	}
	baseURL := "https://" + site
	authorizeURL, err := c.authorizeURL()
	if err != nil {
		return "", err
	}
	launchURL := baseURL + "/admin/tool/mobile/launch.php?service=moodle_mobile_app&passport=1&urlscheme=moodlemobile"
	creds, err := keychain.Load(c.KeychainService, c.KeychainAccount)
	if err != nil {
		return "", err
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	if _, err := cas.Login(ctx, client, authorizeURL, creds.Login(), creds.Password); err != nil {
		return "", err
	}
	token, err := launch(ctx, jar, launchURL, site)
	var netErr *url.Error
	if errors.As(err, &netErr) {
		return "", err // the site was not reached; a later call may try again
	}
	if err != nil {
		return "", &refusedError{site: site, err: err}
	}
	return token, nil
}

// refusedError is a sign-in the SSO accepted and the Moodle site did not complete.
type refusedError struct {
	site string
	err  error
}

func (e *refusedError) Error() string {
	return e.site + " refused the sign-in (not retried until the server restarts): " + e.err.Error()
}

func (e *refusedError) Unwrap() error { return e.err }

// launch asks the mobile app launcher of a site, with the SSO cookies in jar, for a token.
func launch(ctx context.Context, jar http.CookieJar, launchURL, site string) (string, error) {
	noRedirect := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, launchURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	encoded, ok := strings.CutPrefix(resp.Header.Get("Location"), "moodlemobile://token=")
	if !ok {
		return "", fmt.Errorf("%s did not issue a mobile token (HTTP %d)", site, resp.StatusCode)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode mobile token: %w", err)
	}
	parts := strings.Split(string(raw), ":::")
	if len(parts) < 2 || parts[1] == "" {
		return "", errors.New("malformed mobile token")
	}
	return parts[1], nil
}
