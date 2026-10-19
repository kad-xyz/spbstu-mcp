package state

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreFileMode(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "state.json")}
	if err := s.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	if s.Get("k") != "v" {
		t.Fatal("value not stored")
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("state file mode %v, want 0600", info.Mode().Perm())
	}
	_ = s.Set("k", "")
	if s.Get("k") != "" {
		t.Error("empty value must delete the key")
	}
}

// A cookie set for a parent domain must come back for every host of that domain, and only once.
func TestJarKeepsCookieDomains(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "state.json")}
	lms, _ := url.Parse("https://courses.example.ru/login")
	portal, _ := url.Parse("https://example.ru/")
	other, _ := url.Parse("https://other.ru/")

	j := NewJar(s, "jar")
	j.SetCookies(lms, []*http.Cookie{
		{Name: "jwt", Value: "fresh", Domain: ".example.ru", Path: "/", MaxAge: 3600},
		{Name: "csrftoken", Value: "c", Path: "/"},
		{Name: "gone", Value: "x", Path: "/", Expires: time.Now().Add(-time.Hour)},
	})

	reloaded := NewJar(s, "jar")
	names := func(u *url.URL) map[string]int {
		m := map[string]int{}
		for _, c := range reloaded.Cookies(u) {
			m[c.Name]++
		}
		return m
	}
	if got := names(portal); got["jwt"] != 1 || got["csrftoken"] != 0 {
		t.Errorf("portal cookies %v: want the domain cookie only", got)
	}
	if got := names(lms); got["jwt"] != 1 || got["csrftoken"] != 1 || got["gone"] != 0 {
		t.Errorf("lms cookies %v", got)
	}
	if got := names(other); len(got) != 0 {
		t.Errorf("cookies leaked to another site: %v", got)
	}
}
