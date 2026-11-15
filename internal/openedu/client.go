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
	"strings"
	"sync"

	"github.com/kad-xyz/spbstu-mcp/internal/cas"
	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

const (
	PortalURL = "https://openedu.ru"
	LMSURL    = "https://courses.openedu.ru"
	ssoHost   = "sso.openedu.ru"
	ssoURL    = "https://" + ssoHost

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
	loginActionRe = regexp.MustCompile(`"loginAction":\s*"((?:[^"\\]|\\.)+)"|<form[^>]*id="kc-form-login"[^>]*action="([^"]+)"`)
	// Other pages, e.g. a required password change, carry a loginAction too.
	loginPageRe = regexp.MustCompile(`"pageId":\s*"login"|id="kc-form-login"`)
	kcSummaryRe = regexp.MustCompile(`"summary":\s*"((?:[^"\\]|\\.)*)"`)
	kcErrorRe   = regexp.MustCompile(`id="input-error[^"]*"[^>]*>\s*([^<]+?)\s*<`)
	// Keycloakify hands the error of the email and password fields over as functions of
	// kcContext.messagesPerField: existsError answers true, get returns the text.
	kcFieldErrorRe = regexp.MustCompile(`fieldName === 'username' \|\| fieldName === 'password'\s*\)\s*\{\s*return true|id="input-error"`)
	kcFieldTextRe  = regexp.MustCompile(`fieldName === 'username' \|\| fieldName === 'password'\s*\)\s*\{\s*return '((?:[^'\\]|\\.)+)'`)
	// What Keycloak says about the credentials, and about an account it has blocked for a while.
	rejectedRe = regexp.MustCompile(`(?i)invalid (username|email|user credentials|password)|(неправильн|неверн)\S* (имя пользователя|логин|e-?mail|адрес|пароль)`)
	lockedRe   = regexp.MustCompile(`(?i)temporarily|too many|временно|слишком много`)
)

// loadPassword reads the openedu.ru password; tests replace it.
var loadPassword = keychain.Load

// ErrRejected is returned when openedu.ru says the email or password is wrong, and only
// then: the caller drops the stored password on it.
var ErrRejected = errors.New("openedu.ru rejected the email or password")

// ErrLocked is returned when openedu.ru has blocked the account for a while after frequent
// sign-ins. The password may well be right, and another attempt prolongs the block.
var ErrLocked = errors.New("openedu.ru has blocked sign-ins to the account for a while")

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
	// trying is the login SignInAs is checking; it is stored only once it has signed in.
	trying *string
}

func (c *Client) init() {
	if c.client == nil {
		c.client = &http.Client{Jar: state.NewJar(c.State, cookiesKey)}
	}
}

// login is the openedu.ru email the client signs in with, or "" for «Политех»; c.mu is held.
func (c *Client) login() string {
	switch {
	case c.trying != nil:
		return *c.trying
	case c.State == nil:
		return c.email
	}
	return c.State.Get(loginKey)
}

// Login is the openedu.ru email an account's session store signs in with, or "" for «Политех».
func Login(s *state.Store) string { return s.Get(loginKey) }

// SignInAs makes the client sign in with an openedu.ru email and its password from the
// credential store, or through «Политех» when email is "". It signs in once to check, or
// only checks the session when the way stays the same, and returns the openedu.ru username;
// when that fails, the previous way and its sessions stay.
func (c *Client) SignInAs(ctx context.Context, email string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	if email == c.login() {
		return c.username(ctx)
	}
	// Another way of signing in is another openedu.ru account. It is tried on a jar of its
	// own and stored only once it has signed in, so a failed attempt has nothing to undo.
	prev, trial := c.client.Jar, state.NewJar(nil, cookiesKey)
	c.client.Jar, c.trying = trial, &email
	user, err := c.username(ctx)
	c.trying = nil
	if err == nil {
		err = c.adopt(email, trial)
	}
	if err != nil {
		c.client.Jar = prev
		return "", err
	}
	return user, nil
}

// adopt makes login and the sessions of its sign-in the client's own; c.mu is held. The
// login and the cookies reach the store in one write: one without the other would be a
// session of the wrong account.
func (c *Client) adopt(login string, jar *state.Jar) error {
	if c.State == nil {
		c.email = login
		return nil
	}
	if err := c.State.SetAll(map[string]string{loginKey: login, cookiesKey: jar.Snapshot()}); err != nil {
		return err
	}
	c.client.Jar = state.NewJar(c.State, cookiesKey)
	return nil
}

// username asks the LMS who is signed in, signing in first when needed; c.mu is held.
func (c *Client) username(ctx context.Context) (string, error) {
	var me struct {
		Username string `json:"username"`
	}
	body, err := c.lms(ctx, "/api/user/v1/me")
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return "", err
	}
	return me.Username, nil
}

// lmsError is the error of an LMS reply; c.mu is held. "User not enrolled" for a course the
// user sees on openedu.ru may mean the other openedu.ru account, so it says which one this is.
func (c *Client) lmsError(path string, status int, body []byte) error {
	if notEnrolled(body) {
		return fmt.Errorf("courses.openedu.ru%s: HTTP %d: %.200s. %s", path, status, body, accountHint(c.login()))
	}
	return fmt.Errorf("courses.openedu.ru%s: HTTP %d: %.200s", path, status, body)
}

func notEnrolled(body []byte) bool { return bytes.Contains(body, []byte("not enrolled")) }

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
		case !left && notEnrolled(body):
			// Open edX says so with 401 or 403, which here is not a lapsed session: signing
			// in again would cost a sign-in on every course of the other account.
			return nil, c.lmsError(path, status, body)
		case unauthorized && attempt == 0:
			c.refreshJWT(ctx)
		case unauthorized && attempt == 1:
			// The sign-in can end in a redirect loop on /auth/complete/ although the session is
			// established, so the retry decides, not the error.
			signInErr = c.signIn(ctx, lmsLogin, "courses.openedu.ru")
		case signInErr != nil:
			return nil, signInErr
		default:
			return nil, c.lmsError(path, status, body)
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
	action, err := loginAction(page, at)
	if err != nil {
		return fmt.Errorf("%s sign-in: %w", host, err)
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
		return formError(body)
	}
	return errors.New(host + " sign-in as " + email + " stopped at " + end.Host + end.Path + " (finish it once in a browser)")
}

// loginAction finds where the SSO page at the URL at posts its email and password form. The
// password goes only to the openedu.ru SSO over TLS: an address read wrong, or one that
// leads elsewhere, is an error.
func loginAction(page []byte, at *url.URL) (string, error) {
	m := loginActionRe.FindSubmatch(page)
	if m == nil {
		return "", fmt.Errorf("email and password form not found at %s%s", at.Host, at.Path)
	}
	action, ok := html.UnescapeString(string(m[2])), true
	if m[1] != nil {
		action, ok = jsString(string(m[1]))
	}
	u, err := url.Parse(action)
	if !ok || err != nil {
		return "", fmt.Errorf("the address of the email and password form at %s%s is not readable", at.Host, at.Path)
	}
	u = at.ResolveReference(u)
	if u.Scheme != "https" || u.Host != ssoHost {
		return "", fmt.Errorf("the email and password form posts to %s://%s, not to https://%s: the password was not sent", u.Scheme, u.Host, ssoHost)
	}
	return u.String(), nil
}

// jsString decodes the inside of a double-quoted JavaScript string as Keycloak writes it,
// with "\/" for a slash and \u escapes.
func jsString(s string) (string, bool) {
	var out string
	err := json.Unmarshal([]byte(`"`+s+`"`), &out)
	return out, err == nil
}

// formError tells why Keycloak showed its form again. Only an error on the email and
// password fields is a rejection. The form also comes back for an account blocked after
// frequent sign-ins or for a stale page, and neither says anything about the password.
func formError(page []byte) error {
	msg := kcMessage(page)
	switch {
	case lockedRe.MatchString(msg):
		return fmt.Errorf("%w: %s", ErrLocked, msg)
	case kcFieldErrorRe.Match(page) || rejectedRe.MatchString(msg):
		if msg == "" {
			return ErrRejected
		}
		return fmt.Errorf("%w: %s", ErrRejected, msg)
	case msg != "":
		return errors.New("openedu.ru showed the sign-in form again: " + msg)
	}
	return errors.New("openedu.ru showed the sign-in form again without saying why")
}

// kcMessage is the error Keycloak shows on its form, e.g. «Неправильное имя пользователя
// или пароль».
func kcMessage(page []byte) string {
	if m := kcSummaryRe.FindSubmatch(page); m != nil {
		s, ok := jsString(string(m[1]))
		if !ok {
			s = string(m[1])
		}
		return strings.TrimSpace(html.UnescapeString(s))
	}
	if m := kcFieldTextRe.FindSubmatch(page); m != nil {
		// A single-quoted string: its quotes are escaped the other way round.
		raw := strings.ReplaceAll(strings.ReplaceAll(string(m[1]), `\'`, `'`), `"`, `\"`)
		s, ok := jsString(raw)
		if !ok {
			s = string(m[1])
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
