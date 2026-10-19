package keychain

import (
	"os"
	"testing"
)

// TestStoreRoundTrip reads back a credential that CI stored with the platform's own tool
// (cmdkey, secret-tool, security) under the service "spbstu-mcp-test".
func TestStoreRoundTrip(t *testing.T) {
	if os.Getenv("STORE_TEST") == "" {
		t.Skip("set STORE_TEST=1 after storing spbstu-mcp-test / ivanov.ii@edu.spbstu.ru / Пароль-123")
	}
	c, err := Load("spbstu-mcp-test")
	if err != nil {
		t.Fatal(err)
	}
	if c.Account != "ivanov.ii@edu.spbstu.ru" || c.Password != "Пароль-123" {
		t.Fatalf("got account %q, password of %d bytes", c.Account, len(c.Password))
	}
}
