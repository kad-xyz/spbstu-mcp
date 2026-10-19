package openedu

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

// TestLiveLMS reads the enrollments through the LMS API, signing in only when the saved
// sessions have lapsed. Run with SPBSTU_LIVE=1.
func TestLiveLMS(t *testing.T) {
	if os.Getenv("SPBSTU_LIVE") == "" {
		t.Skip("set SPBSTU_LIVE=1 to run against openedu.ru")
	}
	c := &Client{KeychainService: "spbstu", State: state.Default()}
	var out any
	if err := c.API(context.Background(), "/api/enrollment/v1/enrollment", &out); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	t.Log(string(b[:min(len(b), 2500)]))
}
