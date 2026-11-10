package moodle

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

func TestClientSite(t *testing.T) {
	tests := []struct {
		name         string
		site         string
		wantURL      string
		wantToken    string
		wantClient   string
		wantRedirect string
		wantErr      bool
	}{
		{
			name:         "default",
			wantURL:      "https://dl.spbstu.ru",
			wantToken:    "moodle.dl.spbstu.ru.token",
			wantClient:   "5cae4f19-00ac-4910-b731-8e44065ad93d",
			wantRedirect: "https://dl.spbstu.ru/auth/spbstu/redirect.php?auth_service=spbstu_oauth2",
		},
		{
			name:         "humanities",
			site:         "dl-hum.spbstu.ru",
			wantURL:      "https://dl-hum.spbstu.ru",
			wantToken:    "moodle.dl-hum.spbstu.ru.token",
			wantClient:   "ae6fe956-d324-4ad5-88d7-c53b32f94452",
			wantRedirect: "https://dl-hum.spbstu.ru/auth/spbstu/redirect.php?auth_service=spbstu_oauth2",
		},
		{
			name:         "lms",
			site:         "lms.spbstu.ru",
			wantURL:      "https://lms.spbstu.ru",
			wantToken:    "moodle.lms.spbstu.ru.token",
			wantClient:   "94d5f926-5aae-4320-b796-3eb79b4f7cb8",
			wantRedirect: "https://lms.spbstu.ru/auth/spbstu/redirect.php?auth_service=spbstu_oauth2",
		},
		{
			name:    "arbitrary host rejected",
			site:    "example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{Site: tt.site}
			gotURL, urlErr := c.baseURL()
			gotToken, tokenErr := c.tokenKey()
			gotAuthorize, authorizeErr := c.authorizeURL()
			if tt.wantErr {
				if urlErr == nil || tokenErr == nil || authorizeErr == nil {
					t.Fatalf("baseURL error = %v, tokenKey error = %v, authorizeURL error = %v; want all", urlErr, tokenErr, authorizeErr)
				}
				return
			}
			if urlErr != nil || tokenErr != nil || authorizeErr != nil {
				t.Fatalf("baseURL error = %v, tokenKey error = %v, authorizeURL error = %v", urlErr, tokenErr, authorizeErr)
			}
			if gotURL != tt.wantURL {
				t.Errorf("baseURL = %q, want %q", gotURL, tt.wantURL)
			}
			if gotToken != tt.wantToken {
				t.Errorf("tokenKey = %q, want %q", gotToken, tt.wantToken)
			}
			authorize, err := url.Parse(gotAuthorize)
			if err != nil {
				t.Fatalf("parse authorizeURL: %v", err)
			}
			if got := authorize.Query().Get("client_id"); got != tt.wantClient {
				t.Errorf("OAuth client_id = %q, want %q", got, tt.wantClient)
			}
			if got := authorize.Query().Get("redirect_uri"); got != tt.wantRedirect {
				t.Errorf("OAuth redirect_uri = %q, want %q", got, tt.wantRedirect)
			}
		})
	}
}

func TestReady(t *testing.T) {
	store := &state.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	c := &Client{Site: "lms.spbstu.ru", State: store}
	if err := c.Ready(); !errors.Is(err, ErrNoSession) {
		t.Fatalf("Ready without a token = %v", err)
	}
	if err := store.Set("moodle.lms.spbstu.ru.token", "t"); err != nil {
		t.Fatal(err)
	}
	if err := c.Ready(); err != nil {
		t.Fatalf("Ready with a saved token = %v", err)
	}
	legacy := &Client{State: &state.Store{Path: filepath.Join(t.TempDir(), "state.json")}}
	_ = legacy.State.Set("dl.token", "t")
	if err := legacy.Ready(); err != nil {
		t.Fatalf("Ready with the old dl.token = %v", err)
	}
}

// A site that refused a sign-in is not signed in to again: the password would go to the SSO
// once more for nothing.
func TestRefusedSignInIsKept(t *testing.T) {
	refused := &refusedError{site: "lms.spbstu.ru", err: errors.New("no token")}
	c := &Client{Site: "lms.spbstu.ru", KeychainService: "spbstu-mcp-test-none", refused: refused}
	if _, err := c.session(context.Background()); !errors.Is(err, refused) {
		t.Fatalf("session = %v, want the kept refusal", err)
	}
	if err := c.Ready(); !errors.Is(err, refused) {
		t.Fatalf("Ready = %v", err)
	}
}

type downTransport struct{}

func (downTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("i/o timeout")
}

// While a site is down a saved token cannot be checked, and signing in would not help.
func TestUnreachableSiteDoesNotSignIn(t *testing.T) {
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = downTransport{}
	t.Cleanup(func() { http.DefaultClient.Transport = old })
	c := &Client{KeychainService: "spbstu-mcp-test-none", State: &state.Store{Path: filepath.Join(t.TempDir(), "state.json")}}
	_ = c.State.Set("moodle.dl.spbstu.ru.token", "t")
	_, err := c.session(context.Background())
	var netErr *url.Error
	if !errors.As(err, &netErr) {
		t.Fatalf("session = %v, want the network error without a sign-in", err)
	}
}
