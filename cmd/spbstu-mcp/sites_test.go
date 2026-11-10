package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/accounts"
	"github.com/kad-xyz/spbstu-mcp/internal/demo"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

func TestParseSites(t *testing.T) {
	got, err := parseSites(" lms.spbstu.ru, DL.spbstu.ru,lms.spbstu.ru,")
	if err != nil || strings.Join(got, ",") != "lms.spbstu.ru,dl.spbstu.ru" {
		t.Fatalf("parseSites = %v, %v", got, err)
	}
	for _, bad := range []string{"example.com", " , "} {
		if _, err := parseSites(bad); err == nil {
			t.Errorf("parseSites(%q) accepted", bad)
		}
	}
}

// serveStudy serves the dl and study tools for the sessions of a.
func serveStudy(t *testing.T, a *accountSet) *mcp.ClientSession {
	t.Helper()
	s := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	s.AddReceivingMiddleware(a.middleware)
	downloads := t.TempDir()
	registerDL(s, dlProxy{}, downloads)
	registerOverview(s, dlProxy{}, openeduProxy{}, mailProxy{})
	registerStudy(s, dlProxy{}, openeduProxy{}, myProxy{}, downloads)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := s.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	c, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// In -demo only the default site has made-up data; the other enabled ones must not crash.
func TestDemoWithSeveralSites(t *testing.T) {
	a := newAccountSet(nil, "spbstu-mcp-test", true, []string{moodle.DefaultSite, "lms.spbstu.ru"})
	c := serveStudy(t, a)
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"study_overview", nil},
		{"study_subject", map[string]any{"query": "базы данных"}},
		{"study_export_ics", nil},
		{"dl_courses", nil},
	} {
		if _, rest, isErr := call(t, c, tc.tool, tc.args); isErr {
			t.Errorf("%s: %s", tc.tool, rest)
		}
	}
	if _, rest, isErr := call(t, c, "dl_courses", map[string]any{"site": "lms.spbstu.ru"}); !isErr || !strings.Contains(rest, "not enabled") {
		t.Errorf("dl_courses on a site without demo data: %q %v", rest, isErr)
	}
	if _, rest, isErr := call(t, c, "dl_courses", map[string]any{"site": "example.com"}); !isErr {
		t.Errorf("dl_courses on an unknown host: %q", rest)
	}
}

// laterDL moves every demo deadline a year ahead, to interleave with demo.DL.
type laterDL struct{ demo.DL }

func (d laterDL) Deadlines(ctx context.Context, back, ahead int) ([]moodle.Deadline, error) {
	v, err := d.DL.Deadlines(ctx, back, ahead)
	for i := range v {
		v[i].Due = "2999" + v[i].Due[4:]
	}
	return v, err
}

func TestOverviewSkipsSitesWithoutSessionAndSorts(t *testing.T) {
	reg := &accounts.Registry{Dir: t.TempDir(), First: func() (string, error) { return own, nil }}
	sites := []string{"dl-hum.spbstu.ru", moodle.DefaultSite, "lms.spbstu.ru"}
	a := newAccountSet(reg, "spbstu-mcp-test", false, sites)
	store := &state.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	a.sessions[own] = &session{account: own, sites: sites, oe: demo.OpenEdu{}, mail: demo.Mail{}, my: demo.My{},
		dl: map[string]dlService{
			"dl-hum.spbstu.ru": laterDL{},
			moodle.DefaultSite: demo.DL{},
			// No saved token: a summary must not sign in here.
			"lms.spbstu.ru": &moodle.Client{Site: "lms.spbstu.ru", KeychainService: "spbstu-mcp-test-none", State: store},
		}}
	c := serveStudy(t, a)
	_, rest, isErr := call(t, c, "study_overview", nil)
	if isErr {
		t.Fatal(rest)
	}
	var out overview
	if err := json.Unmarshal([]byte(rest), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out.Skipped["lms.spbstu.ru"]; !ok || len(out.Errors) != 0 {
		t.Errorf("skipped = %v, errors = %v", out.Skipped, out.Errors)
	}
	if len(out.Deadlines) == 0 {
		t.Fatal("no deadlines")
	}
	for i := 1; i < len(out.Deadlines); i++ {
		if out.Deadlines[i-1].Due > out.Deadlines[i].Due {
			t.Fatalf("deadlines not sorted: %s before %s", out.Deadlines[i-1].Due, out.Deadlines[i].Due)
		}
	}

	_, rest, _ = call(t, c, "study_export_ics", nil)
	if !strings.Contains(rest, `"errors"`) || !strings.Contains(rest, "lms.spbstu.ru") {
		t.Errorf("study_export_ics hides the site it did not read: %s", rest)
	}
}
