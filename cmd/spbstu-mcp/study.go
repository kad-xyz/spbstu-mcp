package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
	"github.com/kad-xyz/spbstu-mcp/internal/mypoly"
)

// planCode is the study plan code in front of a discipline name, e.g. "Б1.В.1.06 ".
var planCode = regexp.MustCompile(`^[БФ]\d[\p{L}\d.()]*\s+`)

func normName(s string) string {
	return strings.ToLower(strings.TrimSpace(planCode.ReplaceAllString(strings.TrimSpace(s), "")))
}

func failing(grade string) bool {
	g := strings.ToLower(grade)
	for _, bad := range []string{"неуд", "не зач", "незач", "неяв", "недоп", "не явил"} {
		if strings.Contains(g, bad) {
			return true
		}
	}
	return false
}

type debt struct {
	Discipline string `json:"discipline"`
	Control    string `json:"control"`
	Semester   int    `json:"semester"`
	Grade      string `json:"last_grade"`
	Date       string `json:"date,omitempty"`
	Lecturer   string `json:"lecturer,omitempty"`
}

type unmarked struct {
	Discipline string `json:"discipline"`
	Semesters  string `json:"semesters"`
	Control    string `json:"control"`
}

type debtsIn struct {
	CurrentSemester int    `json:"current_semester,omitempty" jsonschema:"the semester now under way; earlier ones are checked (default: the one after the last graded)"`
	RecordBook      string `json:"record_book,omitempty" jsonschema:"record book id; defaults to the first"`
}

type subjectIn struct {
	Query string `json:"query" jsonschema:"part of the discipline name, e.g. базы данных"`
}

type icsIn struct {
	DaysAhead int `json:"days_ahead,omitempty" jsonschema:"how far ahead (default 120)"`
}

func containsAll(s string, words []string) bool {
	s = strings.ToLower(s)
	for _, w := range words {
		if !strings.Contains(s, w) {
			return false
		}
	}
	return true
}

func registerStudy(s *mcp.Server, dl dlService, oe openeduService, my myService, downloads string) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "my_debts", Annotations: readOnly,
		Description: "Academic debts from the record book: disciplines whose latest grade is failing (неуд, не зачтено, неявка). " +
			"Also lists past-semester disciplines of the study plan with no grade in the record book, to check by hand."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in debtsIn) (*mcp.CallToolResult, any, error) {
			grades, err := my.Grades(ctx, in.RecordBook, 0)
			if err != nil {
				return nil, nil, err
			}
			latest := map[string]mypoly.Grade{}
			graded := map[string]bool{}
			last := 0
			for _, g := range grades.Grades {
				key := normName(g.Discipline) + "|" + strings.ToLower(g.Control)
				if prev, ok := latest[key]; !ok || g.Semester > prev.Semester || (g.Semester == prev.Semester && laterDate(g.Date, prev.Date)) {
					latest[key] = g
				}
				graded[normName(g.Discipline)] = true
				last = max(last, g.Semester)
			}
			var debts []debt
			for _, g := range latest {
				if failing(g.Grade) {
					debts = append(debts, debt{Discipline: g.Discipline, Control: g.Control, Semester: g.Semester, Grade: g.Grade, Date: g.Date, Lecturer: g.Lecturer})
				}
			}
			sort.Slice(debts, func(i, j int) bool { return debts[i].Semester < debts[j].Semester })
			current := in.CurrentSemester
			if current <= 0 {
				current = last + 1
			}
			var missing []unmarked
			if plan, err := my.StudyPlan(ctx, in.RecordBook, 0); err == nil {
				for _, d := range plan {
					// Electives not chosen and optional courses (ФТД, факультативы) have no grade by design.
					if d.Group != "" || d.Control == "" || strings.HasPrefix(d.Name, "ФТД") || graded[normName(d.Name)] {
						continue
					}
					past := d.Semesters != ""
					for _, sem := range strings.Split(d.Semesters, ",") {
						var n int
						if _, err := fmt.Sscan(strings.TrimSpace(sem), &n); err != nil || n >= current {
							past = false
						}
					}
					if past {
						missing = append(missing, unmarked{Discipline: d.Name, Semesters: d.Semesters, Control: d.Control})
					}
				}
			}
			return result(map[string]any{"current_semester": current, "debts": debts, "no_grade_in_record_book": missing})
		})

	mcp.AddTool(s, &mcp.Tool{Name: "study_subject", Annotations: readOnly,
		Description: "Everything about one discipline in one call: work-plan hours and control type, the dl course with its " +
			"assignments, quizzes, deadlines and teachers, and the grades already received."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in subjectIn) (*mcp.CallToolResult, any, error) {
			words := strings.Fields(strings.ToLower(in.Query))
			if len(words) == 0 {
				return nil, nil, fmt.Errorf("give part of the discipline name")
			}
			out := map[string]any{}
			errs := map[string]string{}
			var mu sync.Mutex
			set := func(k string, v any, err error) {
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs[k] = err.Error()
					return
				}
				out[k] = v
			}
			var wg sync.WaitGroup
			wg.Go(func() {
				plan, err := my.WorkPlan(ctx, "", 0)
				var hits []mypoly.Discipline
				for sem, ds := range plan {
					for _, d := range ds {
						if containsAll(d.Name, words) {
							d.Semesters = sem
							hits = append(hits, d)
						}
					}
				}
				set("work_plan", hits, err)
			})
			wg.Go(func() {
				g, err := my.Grades(ctx, "", 0)
				var hits []mypoly.Grade
				if err == nil {
					for _, x := range g.Grades {
						if containsAll(x.Discipline, words) {
							hits = append(hits, x)
						}
					}
				}
				set("grades", hits, err)
			})
			wg.Go(func() {
				courses, err := dl.Courses(ctx, false)
				if err != nil {
					set("dl", nil, err)
					return
				}
				deadlines, _ := dl.Deadlines(ctx, 30, 180)
				var cards []map[string]any
				for _, c := range courses {
					if !containsAll(c.Name, words) {
						continue
					}
					card := map[string]any{"course_id": c.ID, "course": c.Name}
					if a, err := dl.Assignments(ctx, c.ID); err == nil {
						var list []map[string]string
						for _, x := range a {
							list = append(list, map[string]string{"name": x.Name, "due": x.Due, "assign_id": fmt.Sprint(x.ID)})
						}
						card["assignments"] = list
					}
					if q, err := dl.Quizzes(ctx, c.ID); err == nil {
						card["quizzes"] = q
					}
					if t, err := dl.Teachers(ctx, c.ID); err == nil {
						card["teachers"] = t
					}
					var due []moodle.Deadline
					for _, d := range deadlines {
						if d.CourseID == c.ID {
							due = append(due, d)
						}
					}
					card["deadlines"] = due
					cards = append(cards, card)
				}
				set("dl_courses", cards, nil)
			})
			wg.Wait()
			if len(errs) > 0 {
				out["errors"] = errs
			}
			return result(out)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "study_export_ics", Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
		Description: "Write every upcoming dl.spbstu.ru deadline and openedu.ru due date into an .ics file (with a reminder a " +
			"day before) and return its path; opening it imports the events into Apple, Google or Outlook Calendar."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in icsIn) (*mcp.CallToolResult, any, error) {
			ahead := orDefault(in.DaysAhead, 120)
			var events []icsEvent
			deadlines, err := dl.Deadlines(ctx, 0, ahead)
			if err != nil {
				return nil, nil, err
			}
			for _, d := range deadlines {
				events = append(events, icsEvent{Title: d.Activity + " — " + d.Course, When: d.Due, URL: d.URL, Note: d.Kind})
			}
			if courses, err := oe.Courses(ctx, true); err == nil {
				for _, c := range courses {
					dates, err := oe.Dates(ctx, c.ID)
					if err != nil {
						continue
					}
					for _, d := range dates {
						if d.Type == "course-start-date" {
							continue
						}
						events = append(events, icsEvent{Title: d.Title + " — " + c.Name + " (openedu)", When: d.Date, URL: d.Link, Note: d.Type})
					}
				}
			}
			path := filepath.Join(downloads, "spbstu-deadlines.ics")
			if err := os.MkdirAll(downloads, 0o755); err != nil {
				return nil, nil, err
			}
			n, err := writeICS(path, events, time.Now().AddDate(0, 0, ahead))
			if err != nil {
				return nil, nil, err
			}
			return result(map[string]any{"path": path, "events": n})
		})
}

// laterDate compares DD.MM.YYYY dates.
func laterDate(a, b string) bool {
	ta, ea := time.Parse("02.01.2006", a)
	tb, eb := time.Parse("02.01.2006", b)
	return ea == nil && (eb != nil || ta.After(tb))
}

type icsEvent struct {
	Title, When, URL, Note string
}

func icsText(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`).Replace(s)
}

// writeICS writes the events due before until, each as a 30-minute slot ending at its due time.
func writeICS(path string, events []icsEvent, until time.Time) (int, error) {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//spbstu-mcp//deadlines//RU\r\nX-WR-CALNAME:СПбПУ: дедлайны\r\n")
	stamp := time.Now().UTC().Format("20060102T150405Z")
	n := 0
	for i, e := range events {
		due, err := time.ParseInLocation("2006-01-02 15:04", e.When, time.Local)
		if err != nil || due.Before(time.Now()) || due.After(until) {
			continue
		}
		start := due.Add(-30 * time.Minute)
		fmt.Fprintf(&b, "BEGIN:VEVENT\r\nUID:spbstu-mcp-%d-%d@spbstu-mcp\r\nDTSTAMP:%s\r\nDTSTART:%s\r\nDTEND:%s\r\nSUMMARY:%s\r\n",
			i, due.Unix(), stamp, start.UTC().Format("20060102T150405Z"), due.UTC().Format("20060102T150405Z"), icsText(e.Title))
		if e.Note != "" {
			fmt.Fprintf(&b, "DESCRIPTION:%s\r\n", icsText(e.Note))
		}
		if e.URL != "" {
			fmt.Fprintf(&b, "URL:%s\r\n", e.URL)
		}
		b.WriteString("BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Скоро срок\r\nTRIGGER:-P1D\r\nEND:VALARM\r\nEND:VEVENT\r\n")
		n++
	}
	b.WriteString("END:VCALENDAR\r\n")
	return n, os.WriteFile(path, []byte(b.String()), 0o644)
}
