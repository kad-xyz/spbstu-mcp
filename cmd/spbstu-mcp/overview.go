package main

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/ews"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
	"github.com/kad-xyz/spbstu-mcp/internal/openedu"
)

type overviewIn struct {
	DaysAhead int `json:"days_ahead,omitempty" jsonschema:"how far ahead to look for deadlines (default 60)"`
}

type oeSummary struct {
	Course     string `json:"course"`
	CourseID   string `json:"course_id"`
	Grade      string `json:"grade"`
	Passing    bool   `json:"passing"`
	Complete   int    `json:"units_complete"`
	Incomplete int    `json:"units_incomplete"`
}

type overview struct {
	Deadlines []moodle.Deadline `json:"dl_deadlines"`
	OpenEdu   []oeSummary       `json:"openedu_courses"`
	Unread    []ews.Message     `json:"unread_mail"`
	Errors    map[string]string `json:"errors,omitempty"`
}

func registerOverview(s *mcp.Server, dl *moodle.Client, oe *openedu.Client, mail *ews.Client) {
	mcp.AddTool(s, &mcp.Tool{Name: "study_overview", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		Description: "One-call summary of the semester: dl.spbstu.ru deadlines (with recently overdue ones), progress " +
			"in current openedu.ru courses and unread university mail. Each source that fails is reported in errors."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in overviewIn) (*mcp.CallToolResult, any, error) {
			ahead := in.DaysAhead
			if ahead <= 0 {
				ahead = 60
			}
			var (
				out overview
				mu  sync.Mutex
				wg  sync.WaitGroup
			)
			fail := func(source string, err error) {
				mu.Lock()
				defer mu.Unlock()
				if out.Errors == nil {
					out.Errors = map[string]string{}
				}
				out.Errors[source] = err.Error()
			}
			wg.Go(func() {
				v, err := dl.Deadlines(ctx, 30, ahead)
				if err != nil {
					fail("dl", err)
					return
				}
				out.Deadlines = v
			})
			wg.Go(func() {
				courses, err := oe.Courses(ctx, true)
				if err != nil {
					fail("openedu", err)
					return
				}
				for _, c := range courses {
					p, err := oe.Progress(ctx, c.ID)
					if err != nil {
						fail("openedu "+c.ID, err)
						continue
					}
					out.OpenEdu = append(out.OpenEdu, oeSummary{Course: c.Name, CourseID: c.ID, Grade: p.Grade,
						Passing: p.Passing, Complete: p.Complete, Incomplete: p.Incomplete})
				}
			})
			wg.Go(func() {
				v, err := mail.List(ctx, "inbox", 30, true, "")
				if err != nil {
					fail("mail", err)
					return
				}
				out.Unread = v
			})
			wg.Wait()
			return result(out)
		})
}
