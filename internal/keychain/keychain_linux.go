package keychain

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// load reads the Secret Service item whose attribute service equals service (and account
// equals account, if given), through the secret-tool CLI of libsecret. Create it with:
//
//	secret-tool store --label=spbstu service spbstu account LOGIN@edu.spbstu.ru
func load(service, account string) (Credentials, error) {
	if err := lookPath(); err != nil {
		return Credentials{}, err
	}
	attrs := attributes(service, account)
	pw, err := run(append([]string{"lookup"}, attrs...)...)
	if err != nil {
		return Credentials{}, fmt.Errorf("Secret Service item %q not found: %w", service, err)
	}
	// search prints the attributes on stderr, so read both streams.
	listing, err := run(append([]string{"search", "--all"}, attrs...)...)
	if err != nil {
		return Credentials{}, fmt.Errorf("Secret Service item %q: %w", service, err)
	}
	return Credentials{Account: parseAccount(listing), Password: strings.TrimRight(pw, "\n")}, nil
}

func lookPath() error {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return errors.New("secret-tool not found: install libsecret-tools (Debian/Ubuntu) or libsecret (Fedora, Arch)")
	}
	return nil
}

func attributes(service, account string) []string {
	if account == "" {
		return []string{"service", service}
	}
	return []string{"service", service, "account", account}
}

func save(string, string, string) error { return ErrUnsupported }

func askPassword(string) (string, error) { return "", ErrUnsupported }

func remove(service, account string) error {
	if err := lookPath(); err != nil {
		return err
	}
	cmd := exec.Command("secret-tool", append([]string{"clear"}, attributes(service, account)...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("delete Secret Service item %q for %s: %s", service, account, strings.TrimSpace(string(out)))
	}
	return nil
}

func storeCommand(service, account string) string {
	return fmt.Sprintf("secret-tool store --label=%s service %s account %s", service, service, account)
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
