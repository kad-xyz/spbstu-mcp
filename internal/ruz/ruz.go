// Package ruz reads the public SPbPU timetable (ruz.spbstu.ru); it needs no sign-in.
package ruz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// attempts and retryDelay: RUZ often answers «Ошибка получения данных … с кэша» for a while; a retry may get the week.
var (
	attempts   = 4
	retryDelay = 2 * time.Second
)

func get(ctx context.Context, path string, out any) error {
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
		}
		var retry bool
		if retry, err = getOnce(ctx, path, out); !retry {
			return err
		}
	}
	return err
}

// getOnce makes one request; retry says the answer was RUZ's own error, worth asking again.
func getOnce(ctx context.Context, path string, out any) (retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+path, nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode >= 500, fmt.Errorf("ruz.spbstu.ru: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	// RUZ reports its failures with HTTP 200 and {"error": true, "text": "..."}.
	var e struct {
		Error bool   `json:"error"`
		Text  string `json:"text"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error {
		return true, fmt.Errorf("ruz.spbstu.ru did not give the timetable: %s", e.Text)
	}
	return false, json.Unmarshal(body, out)
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

// RoomWeek is the timetable of a room for the week containing day, with the groups of every lesson.
// The remote "room" (building 33 "DL", room 967 "Дистанционно") holds the online lessons of the whole university.
func RoomWeek(ctx context.Context, buildingID, roomID int, day time.Time) (*Week, error) {
	return week(ctx, "/buildings/"+strconv.Itoa(buildingID)+"/rooms/"+strconv.Itoa(roomID)+"/scheduler", day, true)
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
		Room *struct {
			Name     string `json:"name"`
			Building struct {
				Name string `json:"name"`
			} `json:"building"`
		} `json:"room"`
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
	} else if raw.Room != nil {
		w.Of = raw.Room.Building.Name + ", ауд. " + raw.Room.Name
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

// Filter keeps the lessons that have every word of q (case-insensitive) in their subject, type, teachers,
// groups, place or note, e.g. "комиссия з5130902/30201".
func (w *Week) Filter(q string) {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return
	}
	kept := []Lesson{}
	for _, l := range w.Lessons {
		text := strings.ToLower(strings.Join(append(append([]string{l.Subject, l.Type, l.Place, l.Note}, l.Teachers...), l.Groups...), " "))
		all := true
		for _, word := range words {
			if !strings.Contains(text, word) {
				all = false
				break
			}
		}
		if all {
			kept = append(kept, l)
		}
	}
	w.Lessons = kept
}

// Kind of timetable a ruz.spbstu.ru page shows.
const (
	KindGroup   = "group"
	KindTeacher = "teacher"
	KindRoom    = "room"
)

// Target is the timetable a ruz.spbstu.ru page link points to.
type Target struct {
	Kind       string
	ID         int    // group, teacher or room id
	BuildingID int    // for a room
	Date       string // the page's ?date=, YYYY-MM-DD, if any
}

// ParseURL reads a ruz.spbstu.ru page link: /faculty/<f>/groups/<id>, /teachers/<id> or /places/<building>/<room>.
func ParseURL(link string) (Target, error) {
	link = strings.TrimSpace(link)
	if !strings.Contains(link, "://") {
		link = "https://" + link
	}
	u, err := url.Parse(link)
	if err != nil || !strings.EqualFold(u.Hostname(), "ruz.spbstu.ru") {
		return Target{}, errors.New("not a ruz.spbstu.ru link")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	num := func(i int) int {
		if i >= len(parts) {
			return 0
		}
		n, _ := strconv.Atoi(parts[i])
		return n
	}
	var t Target
	switch {
	case len(parts) >= 4 && parts[0] == "faculty" && parts[2] == "groups":
		t = Target{Kind: KindGroup, ID: num(3)}
	case len(parts) >= 2 && parts[0] == "teachers":
		t = Target{Kind: KindTeacher, ID: num(1)}
	case len(parts) >= 3 && parts[0] == "places":
		t = Target{Kind: KindRoom, BuildingID: num(1), ID: num(2)}
	}
	if t.Kind == "" || t.ID == 0 || (t.Kind == KindRoom && t.BuildingID == 0) {
		return Target{}, fmt.Errorf("unknown ruz.spbstu.ru page %q: expected a group, teacher or room (/places/…) page", u.Path)
	}
	if d := u.Query().Get("date"); d != "" {
		if day, err := time.Parse("2006-1-2", d); err == nil {
			t.Date = day.Format("2006-01-02")
		}
	}
	return t, nil
}

// TargetWeek is the timetable that a parsed page link points to, for the week containing day.
func TargetWeek(ctx context.Context, t Target, day time.Time) (*Week, error) {
	switch t.Kind {
	case KindGroup:
		return GroupWeek(ctx, t.ID, day)
	case KindTeacher:
		return TeacherWeek(ctx, t.ID, day)
	case KindRoom:
		return RoomWeek(ctx, t.BuildingID, t.ID, day)
	}
	return nil, fmt.Errorf("unknown timetable kind %q", t.Kind)
}
