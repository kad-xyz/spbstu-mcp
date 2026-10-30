package moodle

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Quiz struct {
	ID           int     `json:"quiz_id"`
	CMID         int     `json:"cmid"`
	Name         string  `json:"name"`
	Opens        string  `json:"opens,omitempty"`
	Closes       string  `json:"closes,omitempty"`
	TimeLimitMin int     `json:"time_limit_min,omitempty"`
	Attempts     string  `json:"attempts_allowed"`
	Used         int     `json:"attempts_used"`
	BestGrade    *string `json:"best_grade,omitempty"`
}

// Quizzes lists the quizzes of a course with their schedule, limits and the user's attempts.
func (c *Client) Quizzes(ctx context.Context, courseID int) ([]Quiz, error) {
	var raw struct {
		Quizzes []struct {
			ID        int     `json:"id"`
			CMID      int     `json:"coursemodule"`
			Name      string  `json:"name"`
			TimeOpen  int64   `json:"timeopen"`
			TimeClose int64   `json:"timeclose"`
			TimeLimit int     `json:"timelimit"`
			Attempts  int     `json:"attempts"`
			Grade     float64 `json:"grade"`
		} `json:"quizzes"`
	}
	if err := c.Call(ctx, "mod_quiz_get_quizzes_by_courses", url.Values{"courseids[0]": {strconv.Itoa(courseID)}}, &raw); err != nil {
		return nil, err
	}
	out := make([]Quiz, len(raw.Quizzes))
	var wg sync.WaitGroup
	for i, q := range raw.Quizzes {
		out[i] = Quiz{ID: q.ID, CMID: q.CMID, Name: q.Name, Opens: date(q.TimeOpen), Closes: date(q.TimeClose), TimeLimitMin: q.TimeLimit / 60, Attempts: "без ограничений"}
		if q.Attempts > 0 {
			out[i].Attempts = strconv.Itoa(q.Attempts)
		}
		wg.Go(func() {
			var attempts struct {
				Attempts []struct{} `json:"attempts"`
			}
			if c.Call(ctx, "mod_quiz_get_user_attempts", url.Values{"quizid": {strconv.Itoa(q.ID)}, "status": {"all"}}, &attempts) == nil {
				out[i].Used = len(attempts.Attempts)
			}
			var best struct {
				HasGrade bool    `json:"hasgrade"`
				Grade    float64 `json:"grade"`
			}
			if c.Call(ctx, "mod_quiz_get_user_best_grade", url.Values{"quizid": {strconv.Itoa(q.ID)}}, &best) == nil && best.HasGrade {
				g := fmt.Sprintf("%g / %g", best.Grade, q.Grade)
				out[i].BestGrade = &g
			}
		})
	}
	wg.Wait()
	return out, nil
}

type Person struct {
	ID         int    `json:"user_id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	LastAccess string `json:"last_access,omitempty"`
}

// Teachers lists the teachers of a course; their user_id is what dl_send_message takes.
func (c *Client) Teachers(ctx context.Context, courseID int) ([]Person, error) {
	var raw []struct {
		ID         int    `json:"id"`
		FullName   string `json:"fullname"`
		LastAccess int64  `json:"lastcourseaccess"`
		Roles      []struct {
			ShortName string `json:"shortname"`
		} `json:"roles"`
	}
	params := url.Values{"courseid": {strconv.Itoa(courseID)},
		"options[0][name]": {"userfields"}, "options[0][value]": {"id,fullname,lastcourseaccess,roles"}}
	if err := c.Call(ctx, "core_enrol_get_enrolled_users", params, &raw); err != nil {
		return nil, err
	}
	names := map[string]string{"editingteacher": "преподаватель", "teacher": "ассистент", "manager": "менеджер курса"}
	var out []Person
	for _, u := range raw {
		for _, r := range u.Roles {
			if role, ok := names[r.ShortName]; ok {
				out = append(out, Person{ID: u.ID, Name: u.FullName, Role: role, LastAccess: date(u.LastAccess)})
				break
			}
		}
	}
	return out, nil
}

type Notification struct {
	Time    string `json:"time"`
	Subject string `json:"subject"`
	Text    string `json:"text,omitempty"`
	Read    bool   `json:"read"`
	URL     string `json:"url,omitempty"`
}

// Notifications returns the newest dl notifications: grades, feedback, forum posts, due dates.
func (c *Client) Notifications(ctx context.Context, limit int) ([]Notification, int, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, 0, err
	}
	var raw struct {
		Notifications []struct {
			Subject     string `json:"subject"`
			Small       string `json:"smallmessage"`
			TimeCreated int64  `json:"timecreated"`
			Read        bool   `json:"read"`
			ContextURL  string `json:"contexturl"`
		} `json:"notifications"`
		Unread int `json:"unreadcount"`
	}
	params := url.Values{"useridto": {strconv.Itoa(uid)}, "limit": {strconv.Itoa(limit)}}
	if err := c.Call(ctx, "message_popup_get_popup_notifications", params, &raw); err != nil {
		return nil, 0, err
	}
	out := make([]Notification, 0, len(raw.Notifications))
	for _, n := range raw.Notifications {
		text := Text(n.Small)
		if text == n.Subject {
			text = ""
		}
		out = append(out, Notification{Time: date(n.TimeCreated), Subject: n.Subject, Text: text, Read: n.Read, URL: n.ContextURL})
	}
	return out, raw.Unread, nil
}

type Conversation struct {
	ID      int      `json:"conversation_id"`
	With    []string `json:"with"`
	Unread  int      `json:"unread,omitempty"`
	Last    string   `json:"last_message,omitempty"`
	LastAt  string   `json:"last_at,omitempty"`
	Members []int    `json:"member_ids"`
}

type Message struct {
	From string `json:"from"`
	Time string `json:"time"`
	Text string `json:"text"`
}

// Conversations lists the user's private conversations on dl, newest first.
func (c *Client) Conversations(ctx context.Context, limit int) ([]Conversation, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Conversations []struct {
			ID          int  `json:"id"`
			UnreadCount *int `json:"unreadcount"`
			Members     []struct {
				ID       int    `json:"id"`
				FullName string `json:"fullname"`
			} `json:"members"`
			Messages []struct {
				Text        string `json:"text"`
				TimeCreated int64  `json:"timecreated"`
			} `json:"messages"`
		} `json:"conversations"`
	}
	params := url.Values{"userid": {strconv.Itoa(uid)}, "limitnum": {strconv.Itoa(limit)}}
	if err := c.Call(ctx, "core_message_get_conversations", params, &raw); err != nil {
		return nil, err
	}
	out := []Conversation{}
	for _, cv := range raw.Conversations {
		x := Conversation{ID: cv.ID}
		for _, m := range cv.Members {
			if m.ID != uid {
				x.With = append(x.With, m.FullName)
				x.Members = append(x.Members, m.ID)
			}
		}
		if len(x.With) == 0 {
			continue // the user's own notes ("self-conversation")
		}
		if cv.UnreadCount != nil {
			x.Unread = *cv.UnreadCount
		}
		if len(cv.Messages) > 0 {
			m := cv.Messages[0]
			x.Last, x.LastAt = Text(m.Text), date(m.TimeCreated)
		}
		out = append(out, x)
	}
	return out, nil
}

// ConversationMessages returns the messages of a conversation, oldest first.
func (c *Client) ConversationMessages(ctx context.Context, conversationID int) ([]Message, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Members []struct {
			ID       int    `json:"id"`
			FullName string `json:"fullname"`
		} `json:"members"`
		Messages []struct {
			UserIDFrom  int    `json:"useridfrom"`
			Text        string `json:"text"`
			TimeCreated int64  `json:"timecreated"`
		} `json:"messages"`
	}
	params := url.Values{"currentuserid": {strconv.Itoa(uid)}, "convid": {strconv.Itoa(conversationID)},
		"newest": {"1"}, "limitnum": {"100"}}
	if err := c.Call(ctx, "core_message_get_conversation_messages", params, &raw); err != nil {
		return nil, err
	}
	names := map[int]string{uid: "я"}
	for _, m := range raw.Members {
		if m.ID != uid {
			names[m.ID] = m.FullName
		}
	}
	out := make([]Message, 0, len(raw.Messages))
	for i := len(raw.Messages) - 1; i >= 0; i-- {
		m := raw.Messages[i]
		out = append(out, Message{From: names[m.UserIDFrom], Time: date(m.TimeCreated), Text: Text(m.Text)})
	}
	return out, nil
}

// SendMessage sends a private dl message to a user.
func (c *Client) SendMessage(ctx context.Context, userID int, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("empty message")
	}
	var out []struct {
		MsgID        int    `json:"msgid"`
		ErrorMessage string `json:"errormessage"`
	}
	params := url.Values{"messages[0][touserid]": {strconv.Itoa(userID)}, "messages[0][text]": {text}, "messages[0][textformat]": {"2"}}
	if err := c.Call(ctx, "core_message_send_instant_messages", params, &out); err != nil {
		return err
	}
	if len(out) == 0 || out[0].MsgID <= 0 {
		msg := "not sent"
		if len(out) > 0 && out[0].ErrorMessage != "" {
			msg = out[0].ErrorMessage
		}
		site, _ := c.site()
		return errors.New(site + ": " + msg)
	}
	return nil
}

type Hit struct {
	Site       string `json:"site"`
	Course     string `json:"course"`
	CourseID   int    `json:"course_id"`
	Section    string `json:"section"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	CMID       int    `json:"cmid"`
	InstanceID int    `json:"instance_id"`
	URL        string `json:"url,omitempty"`
	Match      string `json:"match"`
}

// Search looks for words in the names, descriptions and file names of every current course.
func (c *Client) Search(ctx context.Context, query string, all bool) ([]Hit, error) {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil, errors.New("empty query")
	}
	courses, err := c.Courses(ctx, !all)
	if err != nil {
		return nil, err
	}
	var (
		mu   sync.Mutex
		out  []Hit
		seen = map[[2]int]bool{}
		wg   sync.WaitGroup
	)
	site, _ := c.site()
	matches := func(s string) bool {
		s = strings.ToLower(s)
		for _, w := range words {
			if !strings.Contains(s, w) {
				return false
			}
		}
		return true
	}
	for _, co := range courses {
		wg.Go(func() {
			sections, err := c.Contents(ctx, co.ID)
			if err != nil {
				return
			}
			for _, s := range sections {
				for _, m := range s.Modules {
					where := ""
					switch {
					case matches(m.Name):
						where = "name"
					case matches(m.Description):
						where = "description: " + snippet(m.Description, words[0])
					default:
						for _, f := range m.Files {
							if matches(f.Name) {
								where = "file: " + f.Name
								break
							}
						}
					}
					if where == "" {
						continue
					}
					mu.Lock()
					if seen[[2]int{co.ID, m.CMID}] {
						mu.Unlock()
						continue
					}
					seen[[2]int{co.ID, m.CMID}] = true
					out = append(out, Hit{Site: site, Course: co.Name, CourseID: co.ID, Section: s.Name, Name: m.Name, Type: m.Type, CMID: m.CMID, InstanceID: m.InstanceID, URL: m.URL, Match: where})
					mu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	sort.SliceStable(out, func(i, j int) bool { return out[i].Course < out[j].Course })
	return out, nil
}

func snippet(text, word string) string {
	r := []rune(text)
	i := strings.Index(strings.ToLower(text), word)
	if i < 0 {
		return string(r[:min(len(r), 120)])
	}
	start := max(0, len([]rune(text[:i]))-40)
	end := min(len(r), start+140)
	return "…" + string(r[start:end]) + "…"
}
