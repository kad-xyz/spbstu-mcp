package moodle

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	tagRe   = regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</(script|style)>|<[^>]+>`)
	spaceRe = regexp.MustCompile(`[ \t\r\f\v]+`)
	blankRe = regexp.MustCompile(`\n\s*\n+`)
	breakRe = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|li|h[1-6]|tr)>`)
)

// Text turns Moodle's HTML fields into plain text.
func Text(s string) string {
	s = breakRe.ReplaceAllString(s, "\n")
	s = html.UnescapeString(tagRe.ReplaceAllString(s, " "))
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(blankRe.ReplaceAllString(s, "\n"))
}

func date(unix int64) string {
	if unix == 0 {
		return ""
	}
	return time.Unix(unix, 0).Format("2006-01-02 15:04")
}

type Course struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortname,omitempty"`
	Ended     string `json:"ended,omitempty"`
}

// Courses lists the courses the user is enrolled in; current drops those past their end date.
func (c *Client) Courses(ctx context.Context, current bool) ([]Course, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID        int    `json:"id"`
		FullName  string `json:"fullname"`
		ShortName string `json:"shortname"`
		EndDate   int64  `json:"enddate"`
	}
	if err := c.Call(ctx, "core_enrol_get_users_courses", url.Values{"userid": {strconv.Itoa(uid)}}, &raw); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	out := make([]Course, 0, len(raw))
	for _, r := range raw {
		if current && r.EndDate != 0 && r.EndDate < now {
			continue
		}
		out = append(out, Course{ID: r.ID, Name: strings.TrimSpace(r.FullName), ShortName: r.ShortName, Ended: date(r.EndDate)})
	}
	return out, nil
}

type File struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type Module struct {
	CMID        int    `json:"cmid"`
	InstanceID  int    `json:"instance_id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	Files       []File `json:"files,omitempty"`
}

type Section struct {
	Name    string   `json:"name"`
	Summary string   `json:"summary,omitempty"`
	Modules []Module `json:"modules"`
}

// Contents lists a course's sections and their activities, resources and files.
func (c *Client) Contents(ctx context.Context, courseID int) ([]Section, error) {
	var raw []struct {
		Name    string `json:"name"`
		Summary string `json:"summary"`
		Modules []struct {
			ID          int    `json:"id"`
			Instance    int    `json:"instance"`
			Name        string `json:"name"`
			ModName     string `json:"modname"`
			URL         string `json:"url"`
			Description string `json:"description"`
			UserVisible bool   `json:"uservisible"`
			Contents    []struct {
				Type     string `json:"type"`
				FileName string `json:"filename"`
				FileURL  string `json:"fileurl"`
				FileSize int64  `json:"filesize"`
			} `json:"contents"`
		} `json:"modules"`
	}
	if err := c.Call(ctx, "core_course_get_contents", url.Values{"courseid": {strconv.Itoa(courseID)}}, &raw); err != nil {
		return nil, err
	}
	out := make([]Section, 0, len(raw))
	for _, s := range raw {
		sec := Section{Name: s.Name, Summary: Text(s.Summary)}
		for _, m := range s.Modules {
			if !m.UserVisible {
				continue
			}
			mod := Module{CMID: m.ID, InstanceID: m.Instance, Name: m.Name, Type: m.ModName, URL: m.URL, Description: Text(m.Description)}
			for _, f := range m.Contents {
				if f.Type == "file" {
					mod.Files = append(mod.Files, File{Name: f.FileName, URL: f.FileURL, Size: f.FileSize})
				}
			}
			sec.Modules = append(sec.Modules, mod)
		}
		if len(sec.Modules) > 0 || sec.Summary != "" {
			out = append(out, sec)
		}
	}
	return out, nil
}

type Deadline struct {
	Due      string `json:"due"`
	Course   string `json:"course"`
	CourseID int    `json:"course_id"`
	Activity string `json:"activity"`
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	Overdue  bool   `json:"overdue,omitempty"`
}

// Deadlines lists the dated actions (assignment due dates, quiz closings, ...) in a window.
func (c *Client) Deadlines(ctx context.Context, daysBack, daysAhead int) ([]Deadline, error) {
	now := time.Now()
	var raw struct {
		Events []struct {
			Name         string `json:"name"`
			ActivityName string `json:"activityname"`
			ActivityStr  string `json:"activitystr"`
			TimeSort     int64  `json:"timesort"`
			URL          string `json:"url"`
			Overdue      bool   `json:"overdue"`
			Course       struct {
				ID       int    `json:"id"`
				FullName string `json:"fullname"`
			} `json:"course"`
		} `json:"events"`
	}
	params := url.Values{
		"timesortfrom": {strconv.FormatInt(now.AddDate(0, 0, -daysBack).Unix(), 10)},
		"timesortto":   {strconv.FormatInt(now.AddDate(0, 0, daysAhead).Unix(), 10)},
		"limitnum":     {"50"},
	}
	if err := c.Call(ctx, "core_calendar_get_action_events_by_timesort", params, &raw); err != nil {
		return nil, err
	}
	out := make([]Deadline, 0, len(raw.Events))
	for _, e := range raw.Events {
		name := e.ActivityName
		if name == "" {
			name = e.Name
		}
		out = append(out, Deadline{Due: date(e.TimeSort), Course: e.Course.FullName, CourseID: e.Course.ID, Activity: name, Kind: e.ActivityStr, URL: e.URL, Overdue: e.Overdue})
	}
	return out, nil
}

type Assignment struct {
	ID          int    `json:"assign_id"`
	CMID        int    `json:"cmid"`
	Name        string `json:"name"`
	Due         string `json:"due,omitempty"`
	Cutoff      string `json:"cutoff,omitempty"`
	Description string `json:"description,omitempty"`
	Files       []File `json:"files,omitempty"`
}

// Assignments lists the assignments of a course with their task text and attached files.
func (c *Client) Assignments(ctx context.Context, courseID int) ([]Assignment, error) {
	var raw struct {
		Courses []struct {
			Assignments []struct {
				ID               int    `json:"id"`
				CMID             int    `json:"cmid"`
				Name             string `json:"name"`
				DueDate          int64  `json:"duedate"`
				CutoffDate       int64  `json:"cutoffdate"`
				Intro            string `json:"intro"`
				IntroAttachments []struct {
					FileName string `json:"filename"`
					FileURL  string `json:"fileurl"`
					FileSize int64  `json:"filesize"`
				} `json:"introattachments"`
			} `json:"assignments"`
		} `json:"courses"`
	}
	if err := c.Call(ctx, "mod_assign_get_assignments", url.Values{"courseids[0]": {strconv.Itoa(courseID)}}, &raw); err != nil {
		return nil, err
	}
	var out []Assignment
	for _, co := range raw.Courses {
		for _, a := range co.Assignments {
			as := Assignment{ID: a.ID, CMID: a.CMID, Name: a.Name, Due: date(a.DueDate), Cutoff: date(a.CutoffDate), Description: Text(a.Intro)}
			for _, f := range a.IntroAttachments {
				as.Files = append(as.Files, File{Name: f.FileName, URL: f.FileURL, Size: f.FileSize})
			}
			out = append(out, as)
		}
	}
	return out, nil
}

// SubmissionStatus describes the user's submission and grade for an assignment.
func (c *Client) SubmissionStatus(ctx context.Context, assignID int) (map[string]any, error) {
	var raw struct {
		LastAttempt struct {
			Submission struct {
				Status       string `json:"status"`
				TimeModified int64  `json:"timemodified"`
				Plugins      []struct {
					Type      string `json:"type"`
					FileAreas []struct {
						Files []struct {
							FileName string `json:"filename"`
							FileURL  string `json:"fileurl"`
						} `json:"files"`
					} `json:"fileareas"`
					EditorFields []struct {
						Text string `json:"text"`
					} `json:"editorfields"`
				} `json:"plugins"`
			} `json:"submission"`
			GradingStatus string `json:"gradingstatus"`
			CanEdit       bool   `json:"canedit"`
		} `json:"lastattempt"`
		Feedback struct {
			GradeForDisplay string `json:"gradefordisplay"`
			GradedDate      int64  `json:"gradeddate"`
			Plugins         []struct {
				EditorFields []struct {
					Text string `json:"text"`
				} `json:"editorfields"`
			} `json:"plugins"`
		} `json:"feedback"`
	}
	if err := c.Call(ctx, "mod_assign_get_submission_status", url.Values{"assignid": {strconv.Itoa(assignID)}}, &raw); err != nil {
		return nil, err
	}
	sub := raw.LastAttempt.Submission
	var files []string
	var texts []string
	for _, p := range sub.Plugins {
		for _, fa := range p.FileAreas {
			for _, f := range fa.Files {
				files = append(files, f.FileName)
			}
		}
		for _, ef := range p.EditorFields {
			if t := Text(ef.Text); t != "" {
				texts = append(texts, t)
			}
		}
	}
	var comments []string
	for _, p := range raw.Feedback.Plugins {
		for _, ef := range p.EditorFields {
			if t := Text(ef.Text); t != "" {
				comments = append(comments, t)
			}
		}
	}
	return map[string]any{
		"status":           sub.Status,
		"modified":         date(sub.TimeModified),
		"grading_status":   raw.LastAttempt.GradingStatus,
		"can_edit":         raw.LastAttempt.CanEdit,
		"submitted_files":  files,
		"submitted_text":   texts,
		"grade":            Text(raw.Feedback.GradeForDisplay),
		"graded":           date(raw.Feedback.GradedDate),
		"teacher_feedback": comments,
	}, nil
}

type GradeItem struct {
	Item     string `json:"item"`
	Grade    string `json:"grade"`
	Range    string `json:"range,omitempty"`
	Feedback string `json:"feedback,omitempty"`
}

// Grades lists the user's grade items in a course.
func (c *Client) Grades(ctx context.Context, courseID int) ([]GradeItem, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var raw struct {
		UserGrades []struct {
			GradeItems []struct {
				ItemName       string `json:"itemname"`
				ItemType       string `json:"itemtype"`
				GradeFormatted string `json:"gradeformatted"`
				RangeFormatted string `json:"rangeformatted"`
				Feedback       string `json:"feedback"`
			} `json:"gradeitems"`
		} `json:"usergrades"`
	}
	params := url.Values{"courseid": {strconv.Itoa(courseID)}, "userid": {strconv.Itoa(uid)}}
	if err := c.Call(ctx, "gradereport_user_get_grade_items", params, &raw); err != nil {
		return nil, err
	}
	var out []GradeItem
	for _, ug := range raw.UserGrades {
		for _, g := range ug.GradeItems {
			name := g.ItemName
			if g.ItemType == "course" {
				name = "Итог по курсу"
			}
			out = append(out, GradeItem{Item: name, Grade: Text(g.GradeFormatted), Range: g.RangeFormatted, Feedback: Text(g.Feedback)})
		}
	}
	return out, nil
}

type Discussion struct {
	ID      int    `json:"discussion_id"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Created string `json:"created"`
	Replies int    `json:"replies"`
	Message string `json:"message,omitempty"`
}

// Discussions lists the discussions of a forum (its instance id, not the cmid).
func (c *Client) Discussions(ctx context.Context, forumID int) ([]Discussion, error) {
	var raw struct {
		Discussions []struct {
			Discussion int    `json:"discussion"`
			Subject    string `json:"subject"`
			Message    string `json:"message"`
			UserFull   string `json:"userfullname"`
			Created    int64  `json:"created"`
			NumReplies int    `json:"numreplies"`
		} `json:"discussions"`
	}
	params := url.Values{"forumid": {strconv.Itoa(forumID)}, "perpage": {"50"}}
	if err := c.Call(ctx, "mod_forum_get_forum_discussions", params, &raw); err != nil {
		return nil, err
	}
	out := make([]Discussion, 0, len(raw.Discussions))
	for _, d := range raw.Discussions {
		msg := Text(d.Message)
		if r := []rune(msg); len(r) > 500 {
			msg = string(r[:500]) + "…"
		}
		out = append(out, Discussion{ID: d.Discussion, Subject: d.Subject, Author: d.UserFull, Created: date(d.Created), Replies: d.NumReplies, Message: msg})
	}
	return out, nil
}

type Post struct {
	Author  string `json:"author"`
	Created string `json:"created"`
	Subject string `json:"subject"`
	Message string `json:"message"`
	Files   []File `json:"files,omitempty"`
}

// Posts returns every post of a discussion, oldest first.
func (c *Client) Posts(ctx context.Context, discussionID int) ([]Post, error) {
	var raw struct {
		Posts []struct {
			Subject     string `json:"subject"`
			Message     string `json:"message"`
			TimeCreated int64  `json:"timecreated"`
			Author      struct {
				FullName string `json:"fullname"`
			} `json:"author"`
			Attachments []struct {
				FileName string `json:"filename"`
				URL      string `json:"url"`
				FileSize int64  `json:"filesize"`
			} `json:"attachments"`
		} `json:"posts"`
	}
	params := url.Values{"discussionid": {strconv.Itoa(discussionID)}, "sortby": {"created"}, "sortdirection": {"ASC"}}
	if err := c.Call(ctx, "mod_forum_get_discussion_posts", params, &raw); err != nil {
		return nil, err
	}
	out := make([]Post, 0, len(raw.Posts))
	for _, p := range raw.Posts {
		post := Post{Author: p.Author.FullName, Created: date(p.TimeCreated), Subject: p.Subject, Message: Text(p.Message)}
		for _, a := range p.Attachments {
			post.Files = append(post.Files, File{Name: a.FileName, URL: a.URL, Size: a.FileSize})
		}
		out = append(out, post)
	}
	return out, nil
}

// Page returns the text of a "page" resource of a course, found by its cmid.
func (c *Client) Page(ctx context.Context, courseID, cmid int) (string, error) {
	var raw struct {
		Pages []struct {
			CourseModule int    `json:"coursemodule"`
			Name         string `json:"name"`
			Intro        string `json:"intro"`
			Content      string `json:"content"`
		} `json:"pages"`
	}
	if err := c.Call(ctx, "mod_page_get_pages_by_courses", url.Values{"courseids[0]": {strconv.Itoa(courseID)}}, &raw); err != nil {
		return "", err
	}
	for _, p := range raw.Pages {
		if p.CourseModule == cmid {
			return strings.TrimSpace(p.Name + "\n\n" + Text(p.Intro) + "\n\n" + Text(p.Content)), nil
		}
	}
	return "", fmt.Errorf("page with cmid %d not found in course %d", cmid, courseID)
}
