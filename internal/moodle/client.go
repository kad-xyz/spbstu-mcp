// Package moodle talks to dl.spbstu.ru through the Moodle mobile web service API.
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

const tokenKey = "dl.token"

const (
	BaseURL      = "https://dl.spbstu.ru"
	authorizeURL = "https://cas.spbstu.ru/oauth2.0/authorize?client_id=5cae4f19-00ac-4910-b731-8e44065ad93d" +
		"&redirect_uri=https%3A%2F%2Fdl.spbstu.ru%2Fauth%2Fspbstu%2Fredirect.php%3Fauth_service%3Dspbstu_oauth2&response_type=code"
	launchURL = BaseURL + "/admin/tool/mobile/launch.php?service=moodle_mobile_app&passport=1&urlscheme=moodlemobile"
)

// Client is a lazily authenticated Moodle web service client. It signs in through the SPbPU
// SSO with the keychain credentials on first use and again whenever the token stops working.
type Client struct {
	KeychainService string
	// KeychainAccount picks the account of the service to sign in with; "" takes the first one.
	KeychainAccount string
	// State keeps the token between runs; nil keeps it in memory only.
	State *state.Store

	mu     sync.Mutex
	token  string
	userID int
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
	form := url.Values{}
	for k, v := range params {
		form[k] = v
	}
	form.Set("wstoken", token)
	form.Set("wsfunction", function)
	form.Set("moodlewsrestformat", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+"/webservice/rest/server.php", strings.NewReader(form.Encode()))
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
	u, err := url.Parse(fileURL)
	if err != nil || u.Host != "dl.spbstu.ru" {
		return "", fmt.Errorf("not a dl.spbstu.ru file URL: %s", fileURL)
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
	if c.State != nil {
		if token := c.State.Get(tokenKey); token != "" {
			if c.call(ctx, token, "core_webservice_get_site_info", nil, &info) == nil && info.UserID != 0 {
				c.token, c.userID = token, info.UserID
				return token, nil
			}
		}
	}
	token, err := signIn(ctx, c.KeychainService, c.KeychainAccount)
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
		_ = c.State.Set(tokenKey, "")
	}
}

// signIn goes through the SSO and asks the mobile app launcher for a token, which it hands
// back as a moodlemobile:// redirect carrying base64("siteid:::token[:::privatetoken]").
func signIn(ctx context.Context, keychainService, keychainAccount string) (string, error) {
	creds, err := keychain.Load(keychainService, keychainAccount)
	if err != nil {
		return "", err
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	if _, err := cas.Login(ctx, client, authorizeURL, creds.Login(), creds.Password); err != nil {
		return "", err
	}
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
		return "", fmt.Errorf("dl.spbstu.ru did not issue a mobile token (HTTP %d)", resp.StatusCode)
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
