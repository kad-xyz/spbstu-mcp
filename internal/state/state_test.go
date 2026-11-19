package state

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("state file mode %v, want 0600", info.Mode().Perm())
	}
	_ = s.Set("k", "")
	if s.Get("k") != "" {
		t.Error("empty value must delete the key")
	}
}

func TestSetAll(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "state.json")}
	if err := s.SetAll(map[string]string{"a": "1", "b": "2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAll(map[string]string{"a": "", "c": "3"}); err != nil {
		t.Fatal(err)
	}
	if s.Get("a") != "" || s.Get("b") != "2" || s.Get("c") != "3" {
		t.Errorf("a=%q b=%q c=%q", s.Get("a"), s.Get("b"), s.Get("c"))
	}
}

// A jar without a store keeps its cookies in a form a store can take later.
func TestJarSnapshot(t *testing.T) {
	u, _ := url.Parse("https://courses.example.ru/login")
	j := NewJar(nil, "cookies")
	j.SetCookies(u, []*http.Cookie{{Name: "sessionid", Value: "v", Path: "/"}})
	s := &Store{Path: filepath.Join(t.TempDir(), "state.json")}
	if err := s.Set("cookies", j.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if ck := NewJar(s, "cookies").Cookies(u); len(ck) != 1 || ck[0].Value != "v" {
		t.Errorf("cookies read back: %v", ck)
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

// Two servers (say, Claude Code and Claude Desktop) write the same file through their own Stores.
func TestStoreConcurrentProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	errs := make(chan error, 20)
	for range 20 {
		go func() { errs <- (&Store{Path: path}).Set("k", "v") }()
	}
	for range 20 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}
