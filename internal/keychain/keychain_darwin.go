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

// load reads the generic password item named service from the login keychain.
func load(service string) (Credentials, error) {
	attrs, err := run("find-generic-password", "-s", service)
	if err != nil {
		return Credentials{}, fmt.Errorf("keychain item %q not found: %w", service, err)
	}
	m := acctRe.FindStringSubmatch(attrs)
	if m == nil {
		m = []string{"", ""}
	}
	pw, err := run("find-generic-password", "-s", service, "-w")
	if err != nil {
		return Credentials{}, fmt.Errorf("read password of keychain item %q: %w", service, err)
	}
	return Credentials{Account: m[1], Password: decodePassword(strings.TrimRight(pw, "\n"))}, nil
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
