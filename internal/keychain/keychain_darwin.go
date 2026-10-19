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
	return Credentials{Account: m[1], Password: strings.TrimRight(pw, "\n")}, nil
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
