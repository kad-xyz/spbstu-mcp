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
	return s.load()[key]
}

// Set stores value under key; an empty value removes it.
func (s *Store) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.load()
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
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func (s *Store) load() map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(s.Path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}
