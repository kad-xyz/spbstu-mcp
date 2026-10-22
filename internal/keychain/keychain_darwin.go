package keychain

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"
)

var acctRe = regexp.MustCompile(`"acct"<blob>="([^"]*)"`)

// load reads the generic password item named service (and account, if given) from the
// login keychain.
func load(service, account string) (Credentials, error) {
	query := []string{"find-generic-password", "-s", service}
	if account != "" {
		query = append(query, "-a", account)
	}
	attrs, err := run(query...)
	if err != nil {
		return Credentials{}, fmt.Errorf("keychain item %q not found: %w", service, err)
	}
	m := acctRe.FindStringSubmatch(attrs)
	if m == nil {
		m = []string{"", ""}
	}
	pw, err := run(append(query, "-w")...)
	if err != nil {
		return Credentials{}, fmt.Errorf("read password of keychain item %q: %w", service, err)
	}
	return Credentials{Account: m[1], Password: decodePassword(strings.TrimRight(pw, "\n"))}, nil
}

// save adds or updates the item through `security -i`, which reads the command from stdin,
// so the password does not show up in the process list.
func save(service, account, password string) error {
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s\n", quote(service), quote(account), quote(password)))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("store keychain item %q for %s: %s", service, account, strings.TrimSpace(out.String()))
	}
	// security -i reports a failed command on its output but still exits 0: read it back.
	if c, err := load(service, account); err != nil || c.Password != password {
		return fmt.Errorf("store keychain item %q for %s: %s", service, account, strings.TrimSpace(out.String()))
	}
	return nil
}

// quote makes s one argument of `security -i`.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func remove(service, account string) error {
	if _, err := run("delete-generic-password", "-s", service, "-a", account); err != nil {
		return fmt.Errorf("delete keychain item %q for %s: %w", service, account, err)
	}
	return nil
}

const passwordDialog = `on run argv
	display dialog "Пароль учётной записи " & item 1 of argv & " (вставьте из приложения «Пароли»):" default answer "" with hidden answer with title "spbstu-mcp" buttons {"Отмена", "Сохранить"} default button "Сохранить" cancel button "Отмена" with icon note
	return text returned of result
end run`

// askPassword shows a dialog with a hidden field; the password goes from it straight to the
// caller, never to the chat.
func askPassword(account string) (string, error) {
	var out, stderr bytes.Buffer
	cmd := exec.Command("/usr/bin/osascript", "-e", passwordDialog, account)
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "-128") {
			return "", errors.New("the user cancelled the password dialog")
		}
		return "", fmt.Errorf("password dialog: %s", strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}

func storeCommand(service, account string) string {
	return fmt.Sprintf("security add-generic-password -U -s %s -a %s -w", service, account)
}

// decodePassword undoes `security -w`, which prints a password that is not printable ASCII
// (any Cyrillic letter) as hex. A hex string is decoded only when it decodes to non-ASCII
// UTF-8, the one case in which security would have printed hex.
func decodePassword(out string) string {
	if len(out)%2 != 0 {
		return out
	}
	b, err := hex.DecodeString(out)
	if err != nil || !utf8.Valid(b) {
		return out
	}
	for _, c := range b {
		if c >= 0x80 {
			return string(b)
		}
	}
	return out
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
