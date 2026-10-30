package moodle

import (
	"net/url"
	"testing"
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
			name:         "physics and mathematics",
			site:         "dl-phmath.spbstu.ru",
			wantURL:      "https://dl-phmath.spbstu.ru",
			wantToken:    "moodle.dl-phmath.spbstu.ru.token",
			wantClient:   "63a3d37e-5344-11ef-978a-50ebf62c78b1",
			wantRedirect: "https://dl-phmath.spbstu.ru/auth/spbstu/redirect.php?auth_service=spbstu_oauth2",
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
