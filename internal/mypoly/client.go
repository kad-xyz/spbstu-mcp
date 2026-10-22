// Package mypoly reads the student's own pages on my.spbstu.ru ("Мой Политех").
package mypoly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

const (
	BaseURL   = "https://my.spbstu.ru"
	loginURL  = BaseURL + "/accounts/basic-login/"
	cookieKey = "my.cookies"
)

var csrfRe = regexp.MustCompile(`name="csrfmiddlewaretoken" value="([^"]+)"`)

// Client keeps a my.spbstu.ru session and signs in with the keychain credentials whenever
// a page redirects to the login form.
type Client struct {
	KeychainService string
	// KeychainAccount picks the account of the service to sign in with; "" takes the first one.
	KeychainAccount string
	// State keeps the session cookies between runs; nil keeps them in memory only.
	State *state.Store

	mu     sync.Mutex
	client *http.Client
}

// Get returns the body of a page under BaseURL, signing in first when needed.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		c.client = &http.Client{Jar: state.NewJar(c.State, cookieKey)}
	}
	body, loggedOut, err := c.get(ctx, path)
	if err != nil || !loggedOut {
		return body, err
	}
	if err := c.signIn(ctx); err != nil {
		return nil, err
	}
	body, loggedOut, err = c.get(ctx, path)
	if err == nil && loggedOut {
		err = errors.New("my.spbstu.ru: still on the login page after signing in")
	}
	return body, err
}

func (c *Client) get(ctx context.Context, path string) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+path, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	if strings.HasPrefix(resp.Request.URL.Path, "/accounts/") {
		return nil, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("my.spbstu.ru%s: HTTP %d", path, resp.StatusCode)
	}
	return body, false, nil
}

func (c *Client) signIn(ctx context.Context) error {
	creds, err := keychain.Load(c.KeychainService, c.KeychainAccount)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	page, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	m := csrfRe.FindSubmatch(page)
	if m == nil {
		return errors.New("my.spbstu.ru: login form not found")
	}
	form := url.Values{
		"csrfmiddlewaretoken": {string(m[1])},
		"username":            {creds.Login()},
		"password":            {creds.Password},
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", loginURL)
	resp, err = c.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if strings.HasPrefix(resp.Request.URL.Path, "/accounts/") {
		return errors.New("my.spbstu.ru rejected the credentials (or locked the account after too many sign-ins; wait before retrying)")
	}
	return nil
}

// Post sends a JSON request to an AJAX endpoint of a signed-in page and decodes the JSON answer.
func (c *Client) Post(ctx context.Context, path string, payload, out any) error {
	// A GET of the page first signs in when needed and refreshes the CSRF cookie.
	if _, err := c.Get(ctx, "/home/profile/"); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	base, _ := url.Parse(BaseURL)
	csrf := ""
	for _, ck := range c.client.Jar.Cookies(base) {
		if ck.Name == "csrftoken" {
			csrf = ck.Value
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-CSRFToken", csrf)
	req.Header.Set("Referer", BaseURL+path)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("my.spbstu.ru%s: HTTP %d: %.200s", path, resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, out)
}
