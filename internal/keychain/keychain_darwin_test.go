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
