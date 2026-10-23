// Package state persists sessions between runs, so the server signs in only when a session
// has expired: each sign-in counts against the university's lockout of frequent logins.
package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store is a small JSON file of named string values, readable only by the user.
type Store struct {
	Path string

	mu sync.Mutex
}

// Default is the store at ~/.config/spbstu-mcp/state.json.
func Default() *Store {
	home, _ := os.UserHomeDir()
	return &Store{Path: filepath.Join(home, ".config", "spbstu-mcp", "state.json")}
}

// Get returns the value stored under key, or "".
func (s *Store) Get(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, _ := s.load()
	return m[key]
}

// Set stores value under key; an empty value removes it.
func (s *Store) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return err
	}
	if value == "" {
		delete(m, key)
	} else {
		m[key] = value
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(s.Path, b)
}

// WriteFile replaces the file at path with data, readable only by the user. Readers see the
// old or the new content, never a half; concurrent writers, in this process or another, each
// write their own temporary file, and the last rename wins.
func WriteFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// rename moves tmp over path, retrying while Windows holds path, see retry.
func rename(tmp, path string) error {
	return retry(func() error { return os.Rename(tmp, path) })
}

// ReadFile reads a file this package replaces, retrying while Windows holds it, see retry.
func ReadFile(path string) ([]byte, error) {
	var b []byte
	err := retry(func() error {
		var err error
		b, err = os.ReadFile(path)
		return err
	})
	return b, err
}

// retry runs op until it succeeds, fails for another reason than a busy file, or a second
// has passed. Windows refuses to replace a file another process is replacing or reading,
// and to open one while it is being replaced; elsewhere files are never busy.
func retry(op func() error) error {
	for wait := time.Millisecond; ; wait *= 2 {
		err := op()
		if err == nil || !busy(err) || wait > time.Second {
			return err
		}
		time.Sleep(wait)
	}
}

// load reads the stored values. A file that cannot be read is an error, so that Set never
// writes back a map that lost the other values.
func (s *Store) load() (map[string]string, error) {
	m := map[string]string{}
	b, err := ReadFile(s.Path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return m, err
	}
	_ = json.Unmarshal(b, &m)
	return m, nil
}
