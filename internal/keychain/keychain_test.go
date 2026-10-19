package keychain

import "testing"

func TestLogin(t *testing.T) {
	for account, want := range map[string]string{
		"ivanov.ii@edu.spbstu.ru": "ivanov.ii",
		"ivanov.ii":               "ivanov.ii",
	} {
		if got := (Credentials{Account: account}).Login(); got != want {
			t.Errorf("Login(%q) = %q, want %q", account, got, want)
		}
	}
}

func TestAcctRe(t *testing.T) {
	out := `attributes:
    "acct"<blob>="ivanov.ii@edu.spbstu.ru"
    "svce"<blob>="spbstu"`
	if m := acctRe.FindStringSubmatch(out); m == nil || m[1] != "ivanov.ii@edu.spbstu.ru" {
		t.Fatalf("account not parsed: %v", m)
	}
}
