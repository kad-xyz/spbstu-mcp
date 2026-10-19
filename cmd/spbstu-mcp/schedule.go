package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/ruz"
)

type scheduleIn struct {
	Group   string `json:"group,omitempty" jsonschema:"group number, e.g. 5130902/30201 (distance-learning groups start with з); defaults to the -group flag"`
	Teacher string `json:"teacher,omitempty" jsonschema:"teacher's surname or full name, instead of a group"`
	Date    string `json:"date,omitempty" jsonschema:"any day of the wanted week, YYYY-MM-DD (default today)"`
}

type ambiguous struct {
	Note    string      `json:"note"`
	Matches []ruz.Match `json:"matches"`
}

func registerSchedule(s *mcp.Server, defaultGroup string) {
	mcp.AddTool(s, &mcp.Tool{Name: "schedule", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(true)},
		Description: "Weekly timetable of an SPbPU group or teacher from ruz.spbstu.ru: lessons with time, type, teachers, " +
			"room and webinar/LMS links. Distance-learning groups often have lessons only around the session."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in scheduleIn) (*mcp.CallToolResult, any, error) {
			day := time.Now()
			if in.Date != "" {
				d, err := time.ParseInLocation("2006-01-02", in.Date, time.Local)
				if err != nil {
					return nil, nil, errors.New("date must be YYYY-MM-DD")
				}
				day = d
			}
			if in.Teacher != "" {
				matches, err := ruz.SearchTeachers(ctx, in.Teacher)
				if err != nil {
					return nil, nil, err
				}
				m, many := pick(matches, in.Teacher)
				if many != nil {
					return result(many)
				}
				w, err := ruz.TeacherWeek(ctx, m.ID, day)
				if err != nil {
					return nil, nil, err
				}
				return result(w)
			}
			group := in.Group
			if group == "" {
				group = defaultGroup
			}
			if group == "" {
				return nil, nil, errors.New("give a group or a teacher (or start the server with -group)")
			}
			matches, err := ruz.SearchGroups(ctx, group)
			if err != nil {
				return nil, nil, err
			}
			m, many := pick(matches, group)
			if many != nil {
				return result(many)
			}
			w, err := ruz.GroupWeek(ctx, m.ID, day)
			if err != nil {
				return nil, nil, err
			}
			return result(w)
		})
}

// pick returns the exact (case-insensitive) match, the only match, or the candidates to choose from.
func pick(matches []ruz.Match, q string) (ruz.Match, *ambiguous) {
	for _, m := range matches {
		if strings.EqualFold(m.Name, q) {
			return m, nil
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	note := "Several matches; call again with the exact name."
	if len(matches) == 0 {
		note = "Nothing found."
	}
	return ruz.Match{}, &ambiguous{Note: note, Matches: matches}
}
