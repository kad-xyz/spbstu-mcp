package openedu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func localTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Local().Format("2006-01-02 15:04")
}

type Course struct {
	ID     string `json:"course_id"`
	Name   string `json:"name"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
	Active bool   `json:"active"`
}

// Courses lists the user's enrollments; current drops courses that have ended.
func (c *Client) Courses(ctx context.Context, current bool) ([]Course, error) {
	var raw []struct {
		IsActive bool `json:"is_active"`
		Details  struct {
			ID    string `json:"course_id"`
			Name  string `json:"course_name"`
			Start string `json:"course_start"`
			End   string `json:"course_end"`
		} `json:"course_details"`
	}
	if err := c.API(ctx, "/api/enrollment/v1/enrollment", &raw); err != nil {
		return nil, err
	}
	now := time.Now()
	var out []Course
	for _, r := range raw {
		if current {
			if end, err := time.Parse(time.RFC3339, r.Details.End); (err == nil && end.Before(now)) || !r.IsActive {
				continue
			}
		}
		out = append(out, Course{ID: r.Details.ID, Name: r.Details.Name, Start: localTime(r.Details.Start), End: localTime(r.Details.End), Active: r.IsActive})
	}
	return out, nil
}

type Subsection struct {
	Name     string `json:"name"`
	Type     string `json:"assignment_type,omitempty"`
	Earned   string `json:"score,omitempty"`
	Graded   bool   `json:"graded,omitempty"`
	URL      string `json:"url"`
	Progress string `json:"-"`
}

type Section struct {
	Name        string       `json:"section"`
	Subsections []Subsection `json:"subsections"`
}

type Progress struct {
	Complete   int       `json:"units_complete"`
	Incomplete int       `json:"units_incomplete"`
	Grade      string    `json:"grade_percent"`
	Passing    bool      `json:"passing"`
	Policy     []string  `json:"grading_policy"`
	Sections   []Section `json:"sections"`
}

// Progress reports completion, the current grade, the grading policy and per-subsection scores.
func (c *Client) Progress(ctx context.Context, courseID string) (*Progress, error) {
	var raw struct {
		Completion struct {
			Complete   int `json:"complete_count"`
			Incomplete int `json:"incomplete_count"`
		} `json:"completion_summary"`
		Grade struct {
			Percent   float64 `json:"percent"`
			IsPassing bool    `json:"is_passing"`
		} `json:"course_grade"`
		Policy struct {
			Assignments []struct {
				Type      string  `json:"type"`
				Total     float64 `json:"num_total"`
				Droppable int     `json:"num_droppable"`
				Weight    float64 `json:"weight"`
				Passing   float64 `json:"passing_grade"`
			} `json:"assignment_policies"`
		} `json:"grading_policy"`
		Sections []struct {
			Name        string `json:"display_name"`
			Subsections []struct {
				Name     string  `json:"display_name"`
				Type     *string `json:"assignment_type"`
				Graded   bool    `json:"has_graded_assignment"`
				Earned   float64 `json:"num_points_earned"`
				Possible float64 `json:"num_points_possible"`
				URL      string  `json:"url"`
			} `json:"subsections"`
		} `json:"section_scores"`
	}
	if err := c.API(ctx, "/api/course_home/progress/"+url.PathEscape(courseID), &raw); err != nil {
		return nil, err
	}
	p := &Progress{
		Complete: raw.Completion.Complete, Incomplete: raw.Completion.Incomplete,
		Grade: fmt.Sprintf("%.0f%%", raw.Grade.Percent*100), Passing: raw.Grade.IsPassing,
	}
	for _, a := range raw.Policy.Assignments {
		p.Policy = append(p.Policy, fmt.Sprintf("%s: %.0f шт., вес %.0f%%, проходной %.0f%%, можно не сдавать %d",
			a.Type, a.Total, a.Weight*100, a.Passing*100, a.Droppable))
	}
	for _, s := range raw.Sections {
		sec := Section{Name: s.Name}
		for _, ss := range s.Subsections {
			sub := Subsection{Name: ss.Name, Graded: ss.Graded, URL: ss.URL}
			if ss.Type != nil {
				sub.Type = *ss.Type
			}
			if ss.Possible > 0 {
				sub.Earned = fmt.Sprintf("%g/%g", ss.Earned, ss.Possible)
			}
			sec.Subsections = append(sec.Subsections, sub)
		}
		p.Sections = append(p.Sections, sec)
	}
	return p, nil
}

// GradedOnly drops the subsections that carry no grade (videos, notes) and the sections left empty,
// which shrinks the progress of a large course to its tests and exams.
func (p *Progress) GradedOnly() *Progress {
	q := *p
	q.Sections = nil
	for _, s := range p.Sections {
		var subs []Subsection
		for _, ss := range s.Subsections {
			if ss.Graded || ss.Earned != "" {
				subs = append(subs, ss)
			}
		}
		if len(subs) > 0 {
			q.Sections = append(q.Sections, Section{Name: s.Name, Subsections: subs})
		}
	}
	return &q
}

type Date struct {
	Date     string `json:"date"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Complete *bool  `json:"complete,omitempty"`
	Link     string `json:"link,omitempty"`
}

// Dates lists a course's dated items: start and end, assignment due dates.
func (c *Client) Dates(ctx context.Context, courseID string) ([]Date, error) {
	var raw struct {
		Blocks []struct {
			Date           string  `json:"date"`
			Title          string  `json:"title"`
			DateType       string  `json:"date_type"`
			AssignmentType *string `json:"assignment_type"`
			Complete       *bool   `json:"complete"`
			Link           string  `json:"link"`
		} `json:"course_date_blocks"`
	}
	if err := c.API(ctx, "/api/course_home/dates/"+url.PathEscape(courseID), &raw); err != nil {
		return nil, err
	}
	out := make([]Date, 0, len(raw.Blocks))
	for _, b := range raw.Blocks {
		kind := b.DateType
		if b.AssignmentType != nil && *b.AssignmentType != "" {
			kind = *b.AssignmentType
		}
		out = append(out, Date{Date: localTime(b.Date), Title: b.Title, Type: kind, Complete: b.Complete, Link: b.Link})
	}
	return out, nil
}

type OutlineItem struct {
	Name     string `json:"name"`
	Complete bool   `json:"complete"`
	Due      string `json:"due,omitempty"`
	URL      string `json:"url"`
}

type Chapter struct {
	Name  string        `json:"chapter"`
	Items []OutlineItem `json:"items"`
}

// Outline lists the chapters of a course and their subsections with completion and due dates.
func (c *Client) Outline(ctx context.Context, courseID string) ([]Chapter, error) {
	type block struct {
		ID       string   `json:"id"`
		Type     string   `json:"type"`
		Name     string   `json:"display_name"`
		Complete bool     `json:"complete"`
		Due      *string  `json:"due"`
		Children []string `json:"children"`
		URL      string   `json:"lms_web_url"`
	}
	var raw struct {
		CourseBlocks struct {
			Blocks map[string]block `json:"blocks"`
		} `json:"course_blocks"`
	}
	if err := c.API(ctx, "/api/course_home/outline/"+url.PathEscape(courseID), &raw); err != nil {
		return nil, err
	}
	blocks := raw.CourseBlocks.Blocks
	var root *block
	for _, b := range blocks {
		if b.Type == "course" {
			b := b
			root = &b
		}
	}
	if root == nil {
		return nil, fmt.Errorf("course outline of %s is empty", courseID)
	}
	var out []Chapter
	for _, chID := range root.Children {
		ch := blocks[chID]
		chapter := Chapter{Name: ch.Name}
		for _, seqID := range ch.Children {
			seq := blocks[seqID]
			item := OutlineItem{Name: seq.Name, Complete: seq.Complete, URL: seq.URL}
			if seq.Due != nil {
				item.Due = localTime(*seq.Due)
			}
			chapter.Items = append(chapter.Items, item)
		}
		out = append(out, chapter)
	}
	return out, nil
}

// Enroll signs the user up for a course run, as the «Записаться» button does, and returns the
// enrollment as openedu_courses lists it. Enrolling twice is harmless.
func (c *Client) Enroll(ctx context.Context, courseID string) (*Course, error) {
	body := map[string]any{"course_details": map[string]string{"course_id": courseID}}
	if _, err := c.jsonLMS(ctx, "/api/enrollment/v1/enrollment", body); err != nil {
		return nil, fmt.Errorf("not enrolled in %s: %w", courseID, err)
	}
	courses, err := c.Courses(ctx, false)
	if err != nil {
		return nil, err
	}
	for _, co := range courses {
		if co.ID == courseID {
			return &co, nil
		}
	}
	return nil, fmt.Errorf("the LMS accepted the enrollment in %s, but it is not among the user's courses", courseID)
}

// CatalogCourse is a course run found in the openedu.ru catalog.
type CatalogCourse struct {
	ID            string `json:"course_id"`
	Name          string `json:"name"`
	Org           string `json:"org"`
	Start         string `json:"start,omitempty"`
	End           string `json:"end,omitempty"`
	EnrollmentEnd string `json:"enrollment_end,omitempty"`
}

// Search finds course runs in the catalog by words of their name, as the LMS course search does.
func (c *Client) Search(ctx context.Context, query string) ([]CatalogCourse, error) {
	form := url.Values{"search_string": {query}, "page_size": {"50"}, "page_index": {"0"}}
	b, err := c.formLMS(ctx, http.MethodPost, "/search/course_discovery/", form)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Results []struct {
			Data struct {
				ID            string `json:"id"`
				Org           string `json:"org"`
				Start         string `json:"start"`
				End           string `json:"end"`
				EnrollmentEnd string `json:"enrollment_end"`
				Content       struct {
					Name string `json:"display_name"`
				} `json:"content"`
			} `json:"data"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("unexpected search response: %.200s", b)
	}
	out := make([]CatalogCourse, 0, len(raw.Results))
	for _, r := range raw.Results {
		d := r.Data
		out = append(out, CatalogCourse{ID: d.ID, Name: d.Content.Name, Org: d.Org,
			Start: localTime(d.Start), End: localTime(d.End), EnrollmentEnd: localTime(d.EnrollmentEnd)})
	}
	return out, nil
}
