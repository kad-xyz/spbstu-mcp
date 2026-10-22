// Package keychain reads the university login from the operating system's credential store:
// the macOS keychain, the Secret Service on Linux (GNOME Keyring, KWallet) or the Windows
// Credential Manager. Several accounts can be kept under one service, one item per account.
package keychain

import (
	"errors"
	"fmt"
	"strings"
)

// Domain is the mail domain of university accounts.
const Domain = "edu.spbstu.ru"

// ErrUnsupported is returned by Save and AskPassword where the store is filled by hand.
var ErrUnsupported = errors.New("not supported on this system")

// Credentials is the stored university login.
type Credentials struct {
	Account  string
	Password string
}

// Load returns the credentials of account stored under service; an empty account takes the
// first item of the service.
func Load(service, account string) (Credentials, error) {
	c, err := load(service, account)
	if err != nil && account != "" {
		// Stores match the account case-sensitively; the first item may hold it in another case.
		if first, ferr := load(service, ""); ferr == nil && strings.EqualFold(first.Account, account) {
			c, err = first, nil
		}
	}
	if err != nil {
		return Credentials{}, err
	}
	if c.Account == "" {
		return Credentials{}, fmt.Errorf("credential %q has no account", service)
	}
	if account != "" && !strings.EqualFold(c.Account, account) {
		return Credentials{}, fmt.Errorf("credential %q holds %s, not %s", service, c.Account, account)
	}
	if c.Password == "" {
		return Credentials{}, fmt.Errorf("credential %q has an empty password", service)
	}
	return c, nil
}

// Save stores the password of account under service, replacing a stored one.
func Save(service, account, password string) error {
	if password == "" {
		return errors.New("empty password")
	}
	return save(service, account, password)
}

// Delete removes the item of account from service.
func Delete(service, account string) error { return remove(service, account) }

// AskPassword asks the user for the password of account in a system dialog, so that it
// never passes through the chat.
func AskPassword(account string) (string, error) { return askPassword(account) }

// StoreCommand is the command that stores the password of account by hand, for systems
// without Save.
func StoreCommand(service, account string) string { return storeCommand(service, account) }

// Account is the full address of a login given with or without its domain, lower-cased.
func Account(login string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	if login != "" && !strings.Contains(login, "@") {
		login += "@" + Domain
	}
	return login
}

// Login is the account without its mail domain, as the university SSO expects it.
func (c Credentials) Login() string {
	login, _, _ := strings.Cut(c.Account, "@")
	return login
}
