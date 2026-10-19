// Package cas signs in to the SPbPU single sign-on (cas.spbstu.ru).
package cas

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var executionRe = regexp.MustCompile(`name="execution" value="([^"]+)"`)

// Login opens startURL, which redirects to the CAS login form, submits the credentials and
// follows the redirects back to the service. client must carry a cookie jar; the service's
// session cookies end up in it. It returns the URL the redirects ended at.
func Login(ctx context.Context, client *http.Client, startURL, username, password string) (*url.URL, error) {
	resp, err := get(ctx, client, startURL)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.Request.URL.Host != "cas.spbstu.ru" {
		// Still signed in: the SSO went straight back to the service.
		return resp.Request.URL, nil
	}
	m := executionRe.FindSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("CAS login form not found at %s", resp.Request.URL.Redacted())
	}
	form := url.Values{
		"username":    {username},
		"password":    {password},
		"execution":   {string(m[1])},
		"_eventId":    {"submit"},
		"geolocation": {""},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resp.Request.URL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	if resp.Request.URL.Host == "cas.spbstu.ru" {
		return nil, fmt.Errorf("CAS rejected the credentials (HTTP %d)", resp.StatusCode)
	}
	return resp.Request.URL, nil
}

func get(ctx context.Context, client *http.Client, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}
