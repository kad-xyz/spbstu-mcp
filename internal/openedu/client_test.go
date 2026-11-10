package openedu

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

func TestLoginAction(t *testing.T) {
	const want = "https://sso.openedu.ru/realms/openedu/login-actions/authenticate?session_code=s1&execution=e1"
	for _, tc := range []struct{ name, page, want string }{
		{"Keycloakify, as sso.openedu.ru serves it",
			`"url": { "loginAction": "https:\/\/sso.openedu.ru\/realms\/openedu\/login-actions\/authenticate?session_code=s1&execution=e1",`, want},
		{"plain Keycloak theme",
			`<form id="kc-form-login" onsubmit="login.disabled = true; return true;" action="https://sso.openedu.ru/realms/openedu/login-actions/authenticate?session_code=s1&amp;execution=e1" method="post">`, want},
		{"no form", `<a href="/realms/openedu/broker/spbstu/login?client_id=edx">Политех</a>`, ""},
	} {
		if got := loginAction([]byte(tc.page)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestKCMessage(t *testing.T) {
	const want = "Invalid username or password."
	for _, tc := range []struct{ name, page, want string }{
		{"escaped Cyrillic", `"message": { "summary": "\u041d\u0435\u0432\u0435\u0440\u043d\u044b\u0439 \u043f\u0430\u0440\u043e\u043b\u044c.", "type": "error" },`, "Неверный пароль."},
		{"Keycloakify", `"message": { "summary": "Invalid username or password.", "type": "error" },`, want},
		{"plain Keycloak theme", `<span id="input-error" class="pf-c-form__helper-text" aria-live="polite">
                Invalid username or password.
            </span>`, want},
		{"no message", `"loginAction": "https://sso.openedu.ru/x"`, ""},
	} {
		if got := kcMessage([]byte(tc.page)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

const (
	testEmail    = "ivanov.ii@example.com"
	testPassword = "Пароль-123"
	ssoPage      = `<script>const kcContext = { "pageId": "login", "url": { "loginAction": "https:\/\/sso.openedu.ru\/realms\/openedu\/login-actions\/authenticate?session_code=s1&execution=e1&client_id=edx&tab_id=t1" },`
)

// fakeOpenEdu plays the LMS and the Keycloak SSO: the LMS session cookie comes from the
// email and password form only, so a passing test went through it.
type fakeOpenEdu struct {
	t      *testing.T
	posted int
}

func (f *fakeOpenEdu) RoundTrip(r *http.Request) (*http.Response, error) {
	reply := func(status int, body string, header ...string) (*http.Response, error) {
		h := http.Header{}
		for i := 0; i+1 < len(header); i += 2 {
			h.Add(header[i], header[i+1])
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: h, Request: r}, nil
	}
	session := false
	if ck, err := r.Cookie("sessionid"); err == nil && ck.Value == "new" {
		session = true
	}
	switch r.URL.Host + r.URL.Path {
	case "courses.openedu.ru/api/user/v1/me":
		if session {
			return reply(200, `{"username":"ivanov_ii"}`)
		}
		return reply(401, `{"detail":"Authentication credentials were not provided."}`)
	case "courses.openedu.ru/api/course_home/v1/progress/course-v1:spbstu+DATANON+fall_2026":
		return reply(401, `{"developer_message":"User not enrolled."}`)
	case "courses.openedu.ru/login_refresh":
		return reply(401, `{}`)
	case "courses.openedu.ru/auth/login/keycloak/":
		if session {
			return reply(302, "", "Location", "https://courses.openedu.ru/")
		}
		return reply(302, "", "Location", "https://sso.openedu.ru/realms/openedu/protocol/openid-connect/auth?client_id=edx")
	case "sso.openedu.ru/realms/openedu/protocol/openid-connect/auth":
		return reply(200, ssoPage+`"realm": {}};</script>`)
	case "sso.openedu.ru/realms/openedu/login-actions/authenticate":
		f.posted++
		if err := r.ParseForm(); err != nil {
			f.t.Fatal(err)
		}
		if r.URL.Query().Get("execution") != "e1" {
			f.t.Errorf("the form went to %s", r.URL)
		}
		if r.PostForm.Get("username") == testEmail && r.PostForm.Get("password") == testPassword {
			return reply(302, "", "Location", "https://courses.openedu.ru/auth/complete/keycloak/?code=c1")
		}
		if r.PostForm.Get("password") == "expired" {
			return reply(200, strings.Replace(ssoPage, `"pageId": "login"`, `"pageId": "login-update-password"`, 1)+`};</script>`)
		}
		return reply(200, ssoPage+`"message": {"summary": "Invalid username or password.", "type": "error"}};</script>`)
	case "courses.openedu.ru/auth/complete/keycloak/":
		return reply(302, "", "Location", "https://courses.openedu.ru/", "Set-Cookie", "sessionid=new; Path=/; Secure; HttpOnly")
	case "courses.openedu.ru/":
		return reply(200, "<html>dashboard</html>")
	}
	f.t.Errorf("unexpected %s %s", r.Method, r.URL)
	return reply(404, "")
}

// fakeClient is a client with sessions of a «Политех» sign-in, stored in a temporary store,
// that reads password from the credential store.
func fakeClient(t *testing.T, password string) (*Client, *fakeOpenEdu) {
	t.Helper()
	old := loadPassword
	loadPassword = func(service, account string) (keychain.Credentials, error) {
		if service != "spbstu-openedu" || account != testEmail {
			return keychain.Credentials{}, errors.New("no such item")
		}
		return keychain.Credentials{Account: account, Password: password}, nil
	}
	t.Cleanup(func() { loadPassword = old })

	store := &state.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	lms, _ := url.Parse(LMSURL)
	state.NewJar(store, cookiesKey).SetCookies(lms, []*http.Cookie{{Name: "sessionid", Value: "old", Path: "/"}})
	f := &fakeOpenEdu{t: t}
	c := &Client{KeychainService: "spbstu", State: store}
	c.init()
	c.client.Transport = f
	return c, f
}

func TestSignInAsEmail(t *testing.T) {
	c, f := fakeClient(t, testPassword)
	user, err := c.SignInAs(context.Background(), testEmail)
	if err != nil {
		t.Fatal(err)
	}
	if user != "ivanov_ii" || f.posted != 1 {
		t.Fatalf("user %q after %d form posts", user, f.posted)
	}
	if got := c.State.Get(loginKey); got != testEmail {
		t.Errorf("stored login %q", got)
	}
	if cookies := c.State.Get(cookiesKey); strings.Contains(cookies, `"old"`) || !strings.Contains(cookies, `"new"`) {
		t.Errorf("the «Политех» session was kept: %s", cookies)
	}

	// A course of the other openedu.ru account points to openedu_login.
	_, err = c.LMS(context.Background(), "/api/course_home/v1/progress/course-v1:spbstu+DATANON+fall_2026")
	if err == nil || !strings.Contains(err.Error(), "as "+testEmail) || !strings.Contains(err.Error(), "openedu_login") {
		t.Errorf("not enrolled: %v", err)
	}
	if f.posted != 1 {
		t.Errorf("the live session was not reused: %d form posts", f.posted)
	}

	// Choosing the same account again only checks the session.
	if user, err := c.SignInAs(context.Background(), testEmail); err != nil || user != "ivanov_ii" || f.posted != 1 {
		t.Errorf("same login again: %q, %v after %d form posts", user, err, f.posted)
	}
}

func TestSignInAsRejected(t *testing.T) {
	c, f := fakeClient(t, "wrong")
	before := c.State.Get(cookiesKey)
	_, err := c.SignInAs(context.Background(), testEmail)
	if !errors.Is(err, ErrRejected) || !strings.Contains(err.Error(), "Invalid username or password.") {
		t.Fatalf("err = %v, want ErrRejected with the reason", err)
	}
	if f.posted != 1 {
		t.Errorf("%d form posts, want 1", f.posted)
	}
	if got := c.State.Get(loginKey); got != "" {
		t.Errorf("the login stayed switched to %q", got)
	}
	if got := c.State.Get(cookiesKey); got != before {
		t.Errorf("the previous sessions were not restored: %s", got)
	}
	if ck := c.client.Jar.Cookies(&url.URL{Scheme: "https", Host: "courses.openedu.ru", Path: "/"}); len(ck) != 1 || ck[0].Value != "old" {
		t.Errorf("the client lost the previous session: %v", ck)
	}
}

func TestSignInAsRequiredAction(t *testing.T) {
	c, _ := fakeClient(t, "expired")
	_, err := c.SignInAs(context.Background(), testEmail)
	if err == nil || errors.Is(err, ErrRejected) || !strings.Contains(err.Error(), "stopped at sso.openedu.ru") {
		t.Fatalf("err = %v, want a stop on the password change page, not a rejection", err)
	}
	if got := c.State.Get(loginKey); got != "" {
		t.Errorf("the login stayed switched to %q", got)
	}
}

func TestAccountHint(t *testing.T) {
	if h := accountHint(""); !strings.Contains(h, "«Политех»") || !strings.Contains(h, "openedu_login") {
		t.Errorf("hint for «Политех»: %s", h)
	}
}
