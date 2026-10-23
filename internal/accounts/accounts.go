// Package accounts keeps the list of university accounts the server can act as and which
// of them is active. Every account has its own session store, so the sessions of one person
// never serve another.
package accounts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

// Registry is a directory of the form
//
//	active                         the active account
//	accounts/<account>/state.json  the sessions of each account
//	state.json                     the sessions of a single-account version, moved on first use
type Registry struct {
	Dir string
	// First names the account to start with when none is active yet: the one already in the
	// credential store.
	First func() (string, error)
}

// Default is the registry in ~/.config/spbstu-mcp.
func Default(first func() (string, error)) *Registry {
	home, _ := os.UserHomeDir()
	return &Registry{Dir: filepath.Join(home, ".config", "spbstu-mcp"), First: first}
}

// List returns the added accounts in alphabetical order.
func (r *Registry) List() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(r.Dir, "accounts"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []string
	for _, e := range entries {
		if e.IsDir() {
			list = append(list, e.Name())
		}
	}
	slices.Sort(list)
	return list, nil
}

// Has reports whether account was added.
func (r *Registry) Has(account string) bool {
	if !valid(account) {
		return false
	}
	info, err := os.Stat(r.accountDir(account))
	return err == nil && info.IsDir()
}

// Active returns the active account. The first time, it adopts the account of First and
// moves the sessions of a single-account version to it.
func (r *Registry) Active() (string, error) {
	b, err := state.ReadFile(filepath.Join(r.Dir, "active"))
	if err == nil {
		if account := strings.TrimSpace(string(b)); account != "" {
			return account, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if r.First == nil {
		return "", errors.New("no active account: add one with account_add")
	}
	account, err := r.First()
	if err != nil {
		return "", fmt.Errorf("no active account: %w", err)
	}
	account = strings.ToLower(account)
	if err := r.Add(account); err != nil {
		return "", err
	}
	old, moved := filepath.Join(r.Dir, "state.json"), r.Store(account).Path
	if _, err := os.Stat(moved); errors.Is(err, fs.ErrNotExist) {
		if err := os.Rename(old, moved); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return account, r.SetActive(account)
}

// SetActive makes an added account the active one.
func (r *Registry) SetActive(account string) error {
	if !r.Has(account) {
		return fmt.Errorf("account %s is not added", account)
	}
	return state.WriteFile(filepath.Join(r.Dir, "active"), []byte(account+"\n"))
}

// Add adds an account with an empty session store.
func (r *Registry) Add(account string) error {
	if !valid(account) {
		return fmt.Errorf("invalid account %q", account)
	}
	return os.MkdirAll(r.accountDir(account), 0o700)
}

// Remove forgets an account that is not active, with its sessions.
func (r *Registry) Remove(account string) error {
	if !r.Has(account) {
		return fmt.Errorf("account %s is not added", account)
	}
	if active, err := r.Active(); err == nil && active == account {
		return fmt.Errorf("account %s is active: switch to another one first", account)
	}
	return os.RemoveAll(r.accountDir(account))
}

// Store is the session store of account.
func (r *Registry) Store(account string) *state.Store {
	return &state.Store{Path: filepath.Join(r.accountDir(account), "state.json")}
}

func (r *Registry) accountDir(account string) string {
	return filepath.Join(r.Dir, "accounts", account)
}

// valid keeps an account usable as a directory name.
func valid(account string) bool {
	return account != "" && account == filepath.Base(account) && !strings.HasPrefix(account, ".") &&
		!strings.ContainsAny(account, `/\:`)
}
