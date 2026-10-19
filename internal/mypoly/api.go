package mypoly

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var recordBookRe = regexp.MustCompile(`<option value="(\d+)"[^>]*>\s*([^<]+?)\s*</option>`)

// RecordBook is one of the student's record books (зачётные книжки), one per programme.
type RecordBook struct {
	ID    string `json:"record_book"`
	Group string `json:"group"`
}

// RecordBooks lists the student's record books, as the study plan page offers them.
func (c *Client) RecordBooks(ctx context.Context) ([]RecordBook, error) {
	page, err := c.Get(ctx, "/science-and-education/study_plan/")
	if err != nil {
		return nil, err
	}
	s := string(page)
	if i := strings.Index(s, `id="recordBook"`); i >= 0 {
		s = s[i:]
		if j := strings.Index(s, "</select>"); j >= 0 {
			s = s[:j]
		}
	}
	var out []RecordBook
	for _, m := range recordBookRe.FindAllStringSubmatch(s, -1) {
		out = append(out, RecordBook{ID: m[1], Group: m[2]})
	}
	if len(out) == 0 {
		return nil, errors.New("my.spbstu.ru: no record book found")
	}
	return out, nil
}

// pickRecordBook returns id, or the first record book when id is empty.
func (c *Client) pickRecordBook(ctx context.Context, id string) (string, error) {
	if id != "" {
		return id, nil
	}
	books, err := c.RecordBooks(ctx)
	if err != nil {
		return "", err
	}
	return books[0].ID, nil
}

type rawDiscipline struct {
	Name        string          `json:"name"`
	Semesters   string          `json:"semesters"`
	Semester    int             `json:"semester"`
	Hours       float64         `json:"hours"`
	ControlType *string         `json:"control_type"`
	Lec         *float64        `json:"lec"`
	Pract       *float64        `json:"pract"`
	Lab         *float64        `json:"lab"`
	Self        *float64        `json:"self"`
	Children    []rawDiscipline `json:"children"`
}

type rawBlock struct {
	BlockName   string          `json:"block_name"`
	Disciplines []rawDiscipline `json:"disciplines"`
}

// Discipline is a course of the study or work plan; elective groups are flattened, with
// their parent in Group.
type Discipline struct {
	Name      string  `json:"name"`
	Block     string  `json:"block,omitempty"`
	Group     string  `json:"elective_group,omitempty"`
	Semesters string  `json:"semesters,omitempty"`
	Hours     float64 `json:"hours"`
	Control   string  `json:"control,omitempty"`
	Lectures  float64 `json:"lectures_h,omitempty"`
	Practice  float64 `json:"practice_h,omitempty"`
	Labs      float64 `json:"labs_h,omitempty"`
	SelfStudy float64 `json:"self_study_h,omitempty"`
}

func flatten(block string, group string, ds []rawDiscipline, out *[]Discipline) {
	for _, d := range ds {
		if len(d.Children) > 0 {
			flatten(block, d.Name, d.Children, out)
			continue
		}
		x := Discipline{Name: d.Name, Block: block, Group: group, Semesters: d.Semesters, Hours: d.Hours}
		if d.ControlType != nil {
			x.Control = *d.ControlType
		}
		for _, p := range []struct {
			v   *float64
			dst *float64
		}{{d.Lec, &x.Lectures}, {d.Pract, &x.Practice}, {d.Lab, &x.Labs}, {d.Self, &x.SelfStudy}} {
			if p.v != nil {
				*p.dst = *p.v
			}
		}
		*out = append(*out, x)
	}
}

func inSemester(list string, semester int) bool {
	for _, s := range strings.Split(list, ",") {
		if strings.TrimSpace(s) == strconv.Itoa(semester) {
			return true
		}
	}
	return false
}

// StudyPlan lists the disciplines of the whole programme; semester > 0 keeps those taught in it.
func (c *Client) StudyPlan(ctx context.Context, recordBook string, semester int) ([]Discipline, error) {
	rb, err := c.pickRecordBook(ctx, recordBook)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Data []rawBlock `json:"data"`
	}
	if err := c.Post(ctx, "/science-and-education/study_plan/get_study_plan_ajax", map[string]string{"record_book": rb}, &raw); err != nil {
		return nil, err
	}
	var all []Discipline
	for _, b := range raw.Data {
		flatten(b.BlockName, "", b.Disciplines, &all)
	}
	if semester <= 0 {
		return all, nil
	}
	var out []Discipline
	for _, d := range all {
		if inSemester(d.Semesters, semester) {
			out = append(out, d)
		}
	}
	return out, nil
}

// WorkPlan lists the current year's disciplines by semester, with contact and self-study hours;
// semester > 0 keeps that semester only.
func (c *Client) WorkPlan(ctx context.Context, recordBook string, semester int) (map[string][]Discipline, error) {
	rb, err := c.pickRecordBook(ctx, recordBook)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Data map[string][]rawBlock `json:"data"`
	}
	if err := c.Post(ctx, "/science-and-education/work_plan/get_work_plan_ajax", map[string]string{"record_book": rb}, &raw); err != nil {
		return nil, err
	}
	out := map[string][]Discipline{}
	for sem, blocks := range raw.Data {
		if semester > 0 && sem != strconv.Itoa(semester) {
			continue
		}
		var ds []Discipline
		for _, b := range blocks {
			flatten(b.BlockName, "", b.Disciplines, &ds)
		}
		for i := range ds {
			ds[i].Semesters = ""
		}
		out[sem] = ds
	}
	return out, nil
}

type Grade struct {
	Semester   int     `json:"semester"`
	Discipline string  `json:"discipline"`
	Control    string  `json:"control"`
	Grade      string  `json:"grade"`
	Date       string  `json:"date,omitempty"`
	Lecturer   string  `json:"lecturer,omitempty"`
	Hours      float64 `json:"hours,omitempty"`
}

type Grades struct {
	Averages map[string]float64 `json:"average_by_semester"`
	Grades   []Grade            `json:"grades"`
}

// RecordBook returns the record book's grades (semester > 0 keeps that semester) and the
// average grade of every semester.
func (c *Client) Grades(ctx context.Context, recordBook string, semester int) (*Grades, error) {
	rb, err := c.pickRecordBook(ctx, recordBook)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Data struct {
			Chart struct {
				Semester []*float64 `json:"semester"`
			} `json:"chart"`
			FilterSemester []int `json:"filter_semester"`
			Grades         []struct {
				Semester    int     `json:"semester"`
				Discipline  string  `json:"discipline"`
				ControlType string  `json:"control_type"`
				Grade       string  `json:"grade"`
				Date        string  `json:"date"`
				Lecturer    string  `json:"lecturer"`
				Hours       float64 `json:"hours"`
			} `json:"grades"`
		} `json:"data"`
	}
	if err := c.Post(ctx, "/science-and-education/recordbook/get_record_book_data_ajax", map[string]string{"record_book": rb}, &raw); err != nil {
		return nil, err
	}
	out := &Grades{Averages: map[string]float64{}}
	for i, avg := range raw.Data.Chart.Semester {
		if avg != nil && i < len(raw.Data.FilterSemester) {
			out.Averages[strconv.Itoa(raw.Data.FilterSemester[i])] = *avg
		}
	}
	for _, g := range raw.Data.Grades {
		if semester > 0 && g.Semester != semester {
			continue
		}
		out.Grades = append(out.Grades, Grade{Semester: g.Semester, Discipline: g.Discipline, Control: g.ControlType,
			Grade: g.Grade, Date: g.Date, Lecturer: strings.TrimSpace(g.Lecturer), Hours: g.Hours})
	}
	sort.SliceStable(out.Grades, func(i, j int) bool { return out.Grades[i].Semester < out.Grades[j].Semester })
	return out, nil
}
