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

// TestLiveExam reports the state of a timed exam without starting it. Run with SPBSTU_LIVE=1
// and SPBSTU_EXAM set to an exam name of the DATAM course.
func TestLiveExam(t *testing.T) {
	exam := os.Getenv("SPBSTU_EXAM")
	if os.Getenv("SPBSTU_LIVE") == "" || exam == "" {
		t.Skip("set SPBSTU_LIVE=1 and SPBSTU_EXAM to run against openedu.ru")
	}
	c := &Client{KeychainService: "spbstu", State: state.Default()}
	e, err := c.Exam(context.Background(), "course-v1:spbstu+DATAM+fall_2026_spring_2027", exam, false)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(e)
	t.Log(string(b))
}
