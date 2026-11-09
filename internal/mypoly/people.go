package mypoly

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Person is a student or employee found in the university directory of the messenger.
type Person struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Kind is "student" or "employee" ("" when the site gives an unknown type).
	Kind string `json:"kind,omitempty"`
	// Unit is the institute or department, e.g. «ИКНК».
	Unit string `json:"unit,omitempty"`
	// Group is the study group of a student, e.g. «з5130902/30203».
	Group string `json:"group,omitempty"`
	// Info is the site's own line, kept for employees whose post does not split into unit and group.
	Info string `json:"info,omitempty"`
}

type rawPeople struct {
	Results []struct {
		ID       int    `json:"id"`
		FullName string `json:"full_name"`
		Info     string `json:"info"`
		Type     string `json:"type_person"`
	} `json:"results"`
}

// SearchPeople finds students and employees by name or personnel number (at least 3 characters),
// as the «new message» dialog of the my.spbstu.ru messenger does.
func (c *Client) SearchPeople(ctx context.Context, query string) ([]Person, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) < 3 {
		return nil, errors.New("the query needs at least 3 characters")
	}
	var raw rawPeople
	form := url.Values{"q": {query}, "action": {"get_person_list"}}
	if err := c.PostForm(ctx, "/messenger/", form, &raw); err != nil {
		return nil, err
	}
	return parsePeople(raw), nil
}

func parsePeople(raw rawPeople) []Person {
	out := make([]Person, 0, len(raw.Results))
	for _, r := range raw.Results {
		p := Person{ID: r.ID, Name: strings.TrimSpace(r.FullName), Info: strings.TrimSpace(r.Info)}
		switch r.Type {
		case "s":
			p.Kind = "student"
		case "e", "p", "w":
			p.Kind = "employee"
		}
		// Students come as «ИКНК, гр.з5130902/30203».
		if unit, grp, ok := strings.Cut(p.Info, ", гр."); ok {
			p.Unit, p.Group, p.Info = strings.TrimSpace(unit), strings.TrimSpace(grp), ""
		}
		out = append(out, p)
	}
	return out
}
