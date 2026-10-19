// Package keychain reads the university login from the operating system's credential store:
// the macOS keychain, the Secret Service on Linux (GNOME Keyring, KWallet) or the Windows
// Credential Manager.
package keychain

import (
	"fmt"
	"strings"
)

// Credentials is the stored university login.
type Credentials struct {
	Account  string
	Password string
}

// Load returns the credentials stored under service.
func Load(service string) (Credentials, error) {
	c, err := load(service)
	if err != nil {
		return Credentials{}, err
	}
	if c.Account == "" {
		return Credentials{}, fmt.Errorf("credential %q has no account", service)
	}
	if c.Password == "" {
		return Credentials{}, fmt.Errorf("credential %q has an empty password", service)
	}
	return c, nil
}

// Login is the account without its mail domain, as the university SSO expects it.
func (c Credentials) Login() string {
	login, _, _ := strings.Cut(c.Account, "@")
	return login
}
