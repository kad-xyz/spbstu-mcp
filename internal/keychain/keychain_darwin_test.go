package keychain

import "testing"

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
