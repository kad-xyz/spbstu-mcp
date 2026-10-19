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
