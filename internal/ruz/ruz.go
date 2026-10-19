// Package ruz reads the public SPbPU timetable (ruz.spbstu.ru); it needs no sign-in.
package ruz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const BaseURL = "https://ruz.spbstu.ru/api/v1/ruz"

// Match is a group or teacher found by a search.
type Match struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Info string `json:"info,omitempty"`
}

type Lesson struct {
	Date     string   `json:"date"`
	Weekday  string   `json:"weekday"`
	Time     string   `json:"time"`
	Subject  string   `json:"subject"`
	Type     string   `json:"type"`
	Teachers []string `json:"teachers,omitempty"`
	Groups   []string `json:"groups,omitempty"`
	Place    string   `json:"place,omitempty"`
	Note     string   `json:"note,omitempty"`
	Webinar  string   `json:"webinar_url,omitempty"`
	LMS      string   `json:"lms_url,omitempty"`
}

type Week struct {
	Of      string   `json:"of"`
	From    string   `json:"from"`
	To      string   `json:"to"`
	Odd     bool     `json:"odd_week"`
	Lessons []Lesson `json:"lessons"`
}

var weekdays = [...]string{"", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}

func get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ruz.spbstu.ru: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SearchGroups finds groups by (part of) their number, e.g. "5130902/30201".
func SearchGroups(ctx context.Context, q string) ([]Match, error) {
	var raw struct {
		Groups []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Type    string `json:"type"`
			Faculty struct {
				Abbr string `json:"abbr"`
			} `json:"faculty"`
		} `json:"groups"`
	}
	if err := get(ctx, "/search/groups?q="+url.QueryEscape(q), &raw); err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(raw.Groups))
	for _, g := range raw.Groups {
		out = append(out, Match{ID: g.ID, Name: g.Name, Info: strings.TrimSpace(g.Faculty.Abbr + " " + g.Type)})
	}
	return out, nil
}

// SearchTeachers finds teachers by (part of) their name.
func SearchTeachers(ctx context.Context, q string) ([]Match, error) {
	var raw struct {
		Teachers []struct {
			ID       int    `json:"id"`
			FullName string `json:"full_name"`
			Chair    string `json:"chair"`
		} `json:"teachers"`
	}
	if err := get(ctx, "/search/teachers?q="+url.QueryEscape(q), &raw); err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(raw.Teachers))
	for _, t := range raw.Teachers {
		out = append(out, Match{ID: t.ID, Name: t.FullName, Info: t.Chair})
	}
	return out, nil
}

// GroupWeek is the timetable of a group for the week containing day.
func GroupWeek(ctx context.Context, groupID int, day time.Time) (*Week, error) {
	return week(ctx, "/scheduler/"+strconv.Itoa(groupID), day, false)
}

// TeacherWeek is the timetable of a teacher for the week containing day.
func TeacherWeek(ctx context.Context, teacherID int, day time.Time) (*Week, error) {
	return week(ctx, "/teachers/"+strconv.Itoa(teacherID)+"/scheduler", day, true)
}

func week(ctx context.Context, path string, day time.Time, withGroups bool) (*Week, error) {
	var raw struct {
		Week struct {
			Start string `json:"date_start"`
			End   string `json:"date_end"`
			IsOdd bool   `json:"is_odd"`
		} `json:"week"`
		Group *struct {
			Name string `json:"name"`
		} `json:"group"`
		Teacher *struct {
			FullName string `json:"full_name"`
		} `json:"teacher"`
		Days []struct {
			Date    string `json:"date"`
			Weekday int    `json:"weekday"`
			Lessons []struct {
				Subject   string `json:"subject"`
				TimeStart string `json:"time_start"`
				TimeEnd   string `json:"time_end"`
				Info      string `json:"additional_info"`
				TypeObj   struct {
					Name string `json:"name"`
				} `json:"typeObj"`
				Groups []struct {
					Name string `json:"name"`
				} `json:"groups"`
				Teachers []struct {
					FullName string `json:"full_name"`
				} `json:"teachers"`
				Auditories []struct {
					Name     string `json:"name"`
					Building struct {
						Name string `json:"name"`
					} `json:"building"`
				} `json:"auditories"`
				Webinar string `json:"webinar_url"`
				LMS     string `json:"lms_url"`
			} `json:"lessons"`
		} `json:"days"`
	}
	if err := get(ctx, path+"?date="+day.Format("2006-01-02"), &raw); err != nil {
		return nil, err
	}
	w := &Week{From: strings.ReplaceAll(raw.Week.Start, ".", "-"), To: strings.ReplaceAll(raw.Week.End, ".", "-"), Odd: raw.Week.IsOdd, Lessons: []Lesson{}}
	if raw.Group != nil {
		w.Of = raw.Group.Name
	} else if raw.Teacher != nil {
		w.Of = raw.Teacher.FullName
	}
	for _, d := range raw.Days {
		for _, l := range d.Lessons {
			les := Lesson{
				Date: d.Date, Time: l.TimeStart + "–" + l.TimeEnd, Subject: l.Subject, Type: l.TypeObj.Name,
				Note: l.Info, Webinar: l.Webinar, LMS: l.LMS,
			}
			if d.Weekday > 0 && d.Weekday < len(weekdays) {
				les.Weekday = weekdays[d.Weekday]
			}
			for _, t := range l.Teachers {
				les.Teachers = append(les.Teachers, t.FullName)
			}
			if withGroups {
				for _, g := range l.Groups {
					les.Groups = append(les.Groups, g.Name)
				}
			}
			var places []string
			for _, a := range l.Auditories {
				places = append(places, strings.TrimSpace(a.Building.Name+", ауд. "+a.Name))
			}
			les.Place = strings.Join(places, "; ")
			w.Lessons = append(w.Lessons, les)
		}
	}
	return w, nil
}
