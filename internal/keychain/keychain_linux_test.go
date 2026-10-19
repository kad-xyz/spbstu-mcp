package keychain

import "testing"

func TestParseAccount(t *testing.T) {
	listing := `[/org/freedesktop/secrets/collection/login/7]
label = spbstu
secret = hunter2
created = 2026-10-20 00:00:00
modified = 2026-10-20 00:00:00
schema = org.freedesktop.Secret.Generic
attribute.account = ivanov.ii@edu.spbstu.ru
attribute.service = spbstu
`
	if got := parseAccount(listing); got != "ivanov.ii@edu.spbstu.ru" {
		t.Errorf("parseAccount = %q", got)
	}
}
