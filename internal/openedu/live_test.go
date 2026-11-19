package openedu

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

// TestLiveSignInAs signs in with an openedu.ru email and password into a fresh session store
// and lists the current courses of that account. Run with SPBSTU_LIVE=1 and
// SPBSTU_OPENEDU_EMAIL set to an email whose password is stored under spbstu-openedu;
// SPBSTU_OPENEDU_COURSE also reads the progress in that course.
func TestLiveSignInAs(t *testing.T) {
	email := os.Getenv("SPBSTU_OPENEDU_EMAIL")
	if os.Getenv("SPBSTU_LIVE") == "" || email == "" {
		t.Skip("set SPBSTU_LIVE=1 and SPBSTU_OPENEDU_EMAIL to run against openedu.ru")
	}
	ctx := context.Background()
	c := &Client{KeychainService: "spbstu", State: &state.Store{Path: filepath.Join(t.TempDir(), "state.json")}}
	user, err := c.SignInAs(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	courses, err := c.Courses(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("signed in as %s, %d current courses", user, len(courses))
	for _, co := range courses {
		t.Log(co.ID)
	}
	if course := os.Getenv("SPBSTU_OPENEDU_COURSE"); course != "" {
		p, err := c.Progress(ctx, course)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: grade %s, %d units complete, %d to go", course, p.Grade, p.Complete, p.Incomplete)
	}
}
