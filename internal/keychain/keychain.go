// Package keychain reads credentials from the macOS login keychain through the security CLI.
package keychain

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var acctRe = regexp.MustCompile(`"acct"<blob>="([^"]*)"`)

// Credentials is a login stored as a generic password item.
type Credentials struct {
	Account  string
	Password string
}

// Load returns the account and password of the generic password item named service.
func Load(service string) (Credentials, error) {
	attrs, err := run("find-generic-password", "-s", service)
	if err != nil {
		return Credentials{}, fmt.Errorf("keychain item %q not found: %w", service, err)
	}
	m := acctRe.FindStringSubmatch(attrs)
	if m == nil {
		return Credentials{}, fmt.Errorf("keychain item %q has no account", service)
	}
	pw, err := run("find-generic-password", "-s", service, "-w")
	if err != nil {
		return Credentials{}, fmt.Errorf("read password of keychain item %q: %w", service, err)
	}
	pw = strings.TrimRight(pw, "\n")
	if pw == "" {
		return Credentials{}, fmt.Errorf("keychain item %q has an empty password", service)
	}
	return Credentials{Account: m[1], Password: pw}, nil
}

// Login is the account without its mail domain, as the university SSO expects it.
func (c Credentials) Login() string {
	login, _, _ := strings.Cut(c.Account, "@")
	return login
}

func run(args ...string) (string, error) {
	var out, stderr bytes.Buffer
	cmd := exec.Command("/usr/bin/security", args...)
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", errors.New(strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}
