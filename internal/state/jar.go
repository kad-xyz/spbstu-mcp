package state

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Jar is a cookie jar whose cookies outlive the process: it records every cookie a server
// sets, with the URL it came from, so that domain cookies come back as domain cookies.
type Jar struct {
	*cookiejar.Jar
	store *Store
	key   string

	mu      sync.Mutex
	records map[string]record
}

type record struct {
	URL    string       `json:"url"`
	Cookie *http.Cookie `json:"cookie"`
}

// NewJar returns a jar persisted under key in store (in memory only when store is nil).
func NewJar(store *Store, key string) *Jar {
	inner, _ := cookiejar.New(nil)
	j := &Jar{Jar: inner, store: store, key: key, records: map[string]record{}}
	if store == nil {
		return j
	}
	var saved []record
	_ = json.Unmarshal([]byte(store.Get(key)), &saved)
	now := time.Now()
	for _, r := range saved {
		if !r.Cookie.Expires.IsZero() && r.Cookie.Expires.Before(now) {
			continue
		}
		u, err := url.Parse(r.URL)
		if err != nil {
			continue
		}
		inner.SetCookies(u, []*http.Cookie{r.Cookie})
		j.records[id(u, r.Cookie)] = r
	}
	return j
}

func id(u *url.URL, c *http.Cookie) string {
	domain := c.Domain
	if domain == "" {
		domain = u.Host
	}
	return strings.TrimPrefix(domain, ".") + c.Path + "|" + c.Name
}

// SetCookies stores the cookies and persists them.
func (j *Jar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.Jar.SetCookies(u, cookies)
	if j.store == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	origin := (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/"}).String()
	for _, c := range cookies {
		if c.MaxAge < 0 || (!c.Expires.IsZero() && c.Expires.Before(time.Now())) {
			delete(j.records, id(u, c))
			continue
		}
		cc := *c
		if c.MaxAge > 0 {
			cc.Expires = time.Now().Add(time.Duration(c.MaxAge) * time.Second)
			cc.MaxAge = 0
		}
		cc.Raw, cc.Unparsed = "", nil
		j.records[id(u, c)] = record{URL: origin, Cookie: &cc}
	}
	list := make([]record, 0, len(j.records))
	for _, r := range j.records {
		list = append(list, r)
	}
	b, _ := json.Marshal(list)
	_ = j.store.Set(j.key, string(b))
}
