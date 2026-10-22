package keychain

import (
	"os"
	"testing"
)

func TestAcctRe(t *testing.T) {
	out := `attributes:
    "acct"<blob>="ivanov.ii@edu.spbstu.ru"
    "svce"<blob>="spbstu"`
	if m := acctRe.FindStringSubmatch(out); m == nil || m[1] != "ivanov.ii@edu.spbstu.ru" {
		t.Fatalf("account not parsed: %v", m)
	}
}

func TestDecodePassword(t *testing.T) {
	cases := map[string]string{
		"d09fd0b0d180d0bed0bbd18c2d313233": "Пароль-123", // security -w output for a Cyrillic password
		"deadbeef":                         "deadbeef",   // decodes to invalid UTF-8: a real password
		"41424344":                         "41424344",   // decodes to ASCII: security would have printed it raw
		"plain-password":                   "plain-password",
		"abc":                              "abc",
	}
	for in, want := range cases {
		if got := decodePassword(in); got != want {
			t.Errorf("decodePassword(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSaveDelete stores a second account of the CI test service, with characters that
// security -i has to have escaped, reads it back and deletes it.
func TestSaveDelete(t *testing.T) {
	if os.Getenv("STORE_TEST") == "" {
		t.Skip("set STORE_TEST=1 to write to the login keychain")
	}
	const account, password = "petrov.pp@edu.spbstu.ru", `Пар"оль \ 1`
	if err := Save("spbstu-mcp-test", account, password); err != nil {
		t.Fatal(err)
	}
	c, err := Load("spbstu-mcp-test", account)
	if err != nil || c.Password != password {
		t.Fatalf("read back: %v, password of %d bytes", err, len(c.Password))
	}
	if err := Delete("spbstu-mcp-test", account); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("spbstu-mcp-test", account); err == nil {
		t.Fatal("the item is still there")
	}
}
