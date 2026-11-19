package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/ruz"
)

type scheduleIn struct {
	Group   string `json:"group,omitempty" jsonschema:"group number, e.g. 5130902/30201 (distance-learning groups start with з); defaults to the group in the active account's record book, then to the -group flag"`
	Teacher string `json:"teacher,omitempty" jsonschema:"teacher's surname or full name, instead of a group"`
	URL     string `json:"url,omitempty" jsonschema:"a ruz.spbstu.ru page link instead of a group or teacher: a group, a teacher or a room (/places/<building>/<room>, e.g. /places/33/967 is the remote room «Дистанционно» of all online lessons)"`
	Query   string `json:"query,omitempty" jsonschema:"keep only lessons having every word in the subject, type, teachers, groups or place, e.g. «комиссия з5130902/30201»; needed for a busy room"`
	Date    string `json:"date,omitempty" jsonschema:"any day of the first wanted week, YYYY-MM-DD (default today, or the link's date)"`
	Weeks   int    `json:"weeks,omitempty" jsonschema:"how many weeks from that one, 1 to 8 (default 1)"`
}

type ambiguous struct {
	Note    string      `json:"note"`
	Matches []ruz.Match `json:"matches"`
}

// weekResult is one week of a multi-week answer, or the reason RUZ did not give it.
type weekResult struct {
	*ruz.Week
	Missing string `json:"week,omitempty"`
	Error   string `json:"error,omitempty"`
}

// maxLessons keeps a busy room (the remote one has hundreds of lessons a week) from flooding the answer.
const maxLessons = 150

// recordBooks gives the group of the active account's first record book.
type recordBooks func(ctx context.Context) (string, error)

func registerSchedule(s *mcp.Server, my myService, defaultGroup string) {
	ownGroup := func(ctx context.Context) (string, error) {
		books, err := my.RecordBooks(ctx)
		if err != nil {
			return "", err
		}
		if len(books) == 0 || books[0].Group == "" {
			return "", errors.New("no group in the record book")
		}
		return books[0].Group, nil
	}
	mcp.AddTool(s, &mcp.Tool{Name: "schedule", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(true)},
		Description: "Timetable of an SPbPU group, teacher or room from ruz.spbstu.ru, a week or several: lessons with time, " +
			"type (lectures, exams, commissions for debts…), teachers, room and webinar/LMS links. Without group, teacher or url " +
			"it is the user's own group. Commissions for debtors are often listed under other groups: look them up by teacher, " +
			"or in the remote room (url ruz.spbstu.ru/places/33/967) with a query. Distance-learning groups often have lessons only around the session."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in scheduleIn) (*mcp.CallToolResult, any, error) {
			return schedule(ctx, in, ownGroup, defaultGroup)
		})
}

func schedule(ctx context.Context, in scheduleIn, ownGroup recordBooks, defaultGroup string) (*mcp.CallToolResult, any, error) {
	if in.Weeks == 0 {
		in.Weeks = 1
	}
	if in.Weeks < 1 || in.Weeks > 8 {
		return nil, nil, errors.New("weeks must be 1 to 8")
	}
	var (
		target ruz.Target
		err    error
	)
	switch {
	case in.URL != "":
		if target, err = ruz.ParseURL(in.URL); err != nil {
			return nil, nil, err
		}
		if in.Date == "" {
			in.Date = target.Date
		}
	case in.Teacher != "":
		matches, err := ruz.SearchTeachers(ctx, in.Teacher)
		if err != nil {
			return nil, nil, err
		}
		m, many := pick(matches, in.Teacher)
		if many != nil {
			return result(many)
		}
		target = ruz.Target{Kind: ruz.KindTeacher, ID: m.ID}
	default:
		group, err := resolveGroup(ctx, in.Group, ownGroup, defaultGroup)
		if err != nil {
			return nil, nil, err
		}
		matches, err := ruz.SearchGroups(ctx, group)
		if err != nil {
			return nil, nil, err
		}
		m, many := pick(matches, group)
		if many != nil {
			return result(many)
		}
		target = ruz.Target{Kind: ruz.KindGroup, ID: m.ID}
	}

	day := time.Now()
	if in.Date != "" {
		if day, err = time.ParseInLocation("2006-01-02", in.Date, time.Local); err != nil {
			return nil, nil, errors.New("date must be YYYY-MM-DD")
		}
	}
	if in.Weeks == 1 {
		w, err := ruz.TargetWeek(ctx, target, day)
		if err != nil {
			return nil, nil, err
		}
		w.Filter(in.Query)
		return result(trim(w, in.Query))
	}
	out := make([]weekResult, 0, in.Weeks)
	for i := 0; i < in.Weeks; i++ {
		d := day.AddDate(0, 0, 7*i)
		w, err := ruz.TargetWeek(ctx, target, d)
		if err != nil {
			out = append(out, weekResult{Missing: "week of " + d.Format("2006-01-02"), Error: err.Error()})
			continue
		}
		w.Filter(in.Query)
		out = append(out, weekResult{Week: trim(w, in.Query)})
	}
	return result(out)
}

// resolveGroup is the asked group, else the group of the active account's record book, else the -group flag.
func resolveGroup(ctx context.Context, asked string, ownGroup recordBooks, defaultGroup string) (string, error) {
	if asked != "" {
		return asked, nil
	}
	own, err := ownGroup(ctx)
	switch {
	case err == nil:
		return own, nil
	case defaultGroup != "":
		return defaultGroup, nil
	}
	return "", fmt.Errorf("give a group, a teacher or a url: the group of the record book is unknown (%v)", err)
}

// trim cuts a week down to maxLessons lessons and says so in the week's name.
func trim(w *ruz.Week, query string) *ruz.Week {
	if len(w.Lessons) <= maxLessons {
		return w
	}
	hint := "give a query to narrow them"
	if query != "" {
		hint = "narrow the query"
	}
	w.Of = fmt.Sprintf("%s (first %d of %d lessons; %s)", w.Of, maxLessons, len(w.Lessons), hint)
	w.Lessons = w.Lessons[:maxLessons]
	return w
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
