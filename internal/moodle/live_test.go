package moodle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

// TestLiveUploadDraft uploads a file to the user's draft area, which publishes nothing.
// Run with SPBSTU_LIVE=1.
func TestLiveUploadDraft(t *testing.T) {
	if os.Getenv("SPBSTU_LIVE") == "" {
		t.Skip("set SPBSTU_LIVE=1 to run against dl.spbstu.ru")
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	_ = os.WriteFile(a, []byte("a"), 0o644)
	_ = os.WriteFile(b, []byte("b"), 0o644)
	c := &Client{KeychainService: "spbstu", State: state.Default()}
	id, err := c.UploadDraft(context.Background(), []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("no draft item id")
	}
	t.Logf("draft item id %d", id)
}
