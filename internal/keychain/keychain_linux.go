package keychain

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// load reads the Secret Service item whose attribute service equals service, through the
// secret-tool CLI of libsecret. Create it with:
//
//	secret-tool store --label=spbstu service spbstu account LOGIN@edu.spbstu.ru
func load(service string) (Credentials, error) {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return Credentials{}, errors.New("secret-tool not found: install libsecret-tools (Debian/Ubuntu) or libsecret (Fedora, Arch)")
	}
	pw, err := run("lookup", "service", service)
	if err != nil {
		return Credentials{}, fmt.Errorf("Secret Service item %q not found: %w", service, err)
	}
	// search prints the attributes on stderr, so read both streams.
	listing, err := run("search", "--all", "service", service)
	if err != nil {
		return Credentials{}, fmt.Errorf("Secret Service item %q: %w", service, err)
	}
	return Credentials{Account: parseAccount(listing), Password: strings.TrimRight(pw, "\n")}, nil
}

// parseAccount finds the account attribute in `secret-tool search` output.
func parseAccount(listing string) string {
	sc := bufio.NewScanner(strings.NewReader(listing))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "attribute.account = "); ok {
			return v
		}
	}
	return ""
}

func run(args ...string) (string, error) {
	var out, stderr bytes.Buffer
	cmd := exec.Command("secret-tool", args...)
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	if out.Len() == 0 && stderr.Len() == 0 {
		return "", errors.New("no such item")
	}
	return out.String() + stderr.String(), nil
}
