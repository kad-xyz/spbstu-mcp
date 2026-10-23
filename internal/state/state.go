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

// rename moves tmp over path. Windows refuses to replace a file while another writer is
// replacing it or a reader has it open, so a refused rename is retried for a while.
func rename(tmp, path string) error {
	var err error
	for wait := time.Millisecond; ; wait *= 2 {
		if err = os.Rename(tmp, path); err == nil || !errors.Is(err, fs.ErrPermission) || wait > time.Second {
			return err
		}
		time.Sleep(wait)
	}
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
