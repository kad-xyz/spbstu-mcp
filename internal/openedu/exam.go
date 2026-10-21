package openedu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Exam is a timed exam of a course with the state of the user's attempt and, while the
// attempt runs, its questions.
type Exam struct {
	ID          int       `json:"exam_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	TimeLimit   string    `json:"time_limit"`
	Due         string    `json:"due,omitempty"`
	AttemptID   int       `json:"attempt_id,omitempty"`
	Status      string    `json:"status"` // not started, started, submitted, …
	MinutesLeft int       `json:"minutes_left,omitempty"`
	Problems    []Problem `json:"problems,omitempty"`
	Notice      string    `json:"notice,omitempty"`
}

type examState struct {
	Exam struct {
		ID        int    `json:"id"`
		Name      string `json:"exam_name"`
		Type      string `json:"type"`
		TimeLimit int    `json:"time_limit_mins"`
		Due       string `json:"due_date"`
		Attempt   struct {
			ID            int     `json:"attempt_id"`
			Status        string  `json:"attempt_status"`
			TimeRemaining float64 `json:"time_remaining_seconds"`
		} `json:"attempt"`
	} `json:"exam"`
}

// Exam reports a timed exam and the user's attempt. With start, an exam not yet started is
// started, which runs its clock out for good; a running attempt comes back with its questions.
func (c *Client) Exam(ctx context.Context, courseID, block string, start bool) (*Exam, error) {
	seq, err := c.examSequence(ctx, courseID, block)
	if err != nil {
		return nil, err
	}
	st, err := c.examState(ctx, courseID, seq)
	if err != nil {
		return nil, err
	}
	if start && st.Exam.Attempt.ID == 0 {
		form := url.Values{"exam_id": {strconv.Itoa(st.Exam.ID)}, "start_clock": {"true"}}
		if _, err := c.formLMS(ctx, http.MethodPost, "/api/edx_proctoring/v1/proctored_exam/attempt", form); err != nil {
			return nil, fmt.Errorf("exam not started: %w", err)
		}
		if st, err = c.examState(ctx, courseID, seq); err != nil {
			return nil, err
		}
	}
	e := &Exam{
		ID: st.Exam.ID, Name: st.Exam.Name, Type: st.Exam.Type, TimeLimit: fmt.Sprintf("%d мин.", st.Exam.TimeLimit), Due: localTime(st.Exam.Due),
		AttemptID: st.Exam.Attempt.ID, Status: st.Exam.Attempt.Status,
	}
	switch {
	case e.AttemptID == 0:
		e.Status = "not started"
		e.Notice = "starting runs the clock for " + e.TimeLimit + " and cannot be undone"
		return e, nil
	case e.Status != "started":
		return e, nil
	}
	e.MinutesLeft = int(st.Exam.Attempt.TimeRemaining / 60)
	var sq struct {
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"page_title"`
		} `json:"items"`
	}
	if err := c.API(ctx, "/api/courseware/sequence/"+seq, &sq); err != nil {
		return nil, err
	}
	for _, it := range sq.Items {
		body, err := c.LMS(ctx, "/xblock/"+it.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range parseProblems(body) {
			p.Unit = it.Title
			e.Problems = append(e.Problems, p)
		}
	}
	return e, nil
}

// FinishExam submits the running attempt of a timed exam, ending its clock.
func (c *Client) FinishExam(ctx context.Context, courseID, block string) (*Exam, error) {
	seq, err := c.examSequence(ctx, courseID, block)
	if err != nil {
		return nil, err
	}
	st, err := c.examState(ctx, courseID, seq)
	if err != nil {
		return nil, err
	}
	if st.Exam.Attempt.ID == 0 || st.Exam.Attempt.Status != "started" {
		return nil, fmt.Errorf("%s has no running attempt (status %q)", st.Exam.Name, st.Exam.Attempt.Status)
	}
	path := "/api/edx_proctoring/v1/proctored_exam/attempt/" + strconv.Itoa(st.Exam.Attempt.ID)
	if _, err := c.formLMS(ctx, http.MethodPut, path, url.Values{"action": {"submit"}}); err != nil {
		return nil, err
	}
	return c.Exam(ctx, courseID, seq, false)
}

func (c *Client) examState(ctx context.Context, courseID, seq string) (*examState, error) {
	// A refused exam (a proctored one on the free track) answers 403, which LMS would take for
	// a lapsed session, so the session is checked first and the exam read without signing in.
	var me struct{}
	if err := c.API(ctx, "/api/user/v1/me", &me); err != nil {
		return nil, err
	}
	path := "/api/edx_proctoring/v1/proctored_exam/attempt/course_id/" + url.PathEscape(courseID) + "?content_id=" + url.QueryEscape(seq)
	c.mu.Lock()
	body, status, _, err := c.fetch(ctx, LMSURL+path)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		var msg []string
		if json.Unmarshal(body, &msg) == nil && len(msg) > 0 {
			return nil, fmt.Errorf("exam refused: %s", strings.Join(msg, " "))
		}
		return nil, fmt.Errorf("courses.openedu.ru%s: HTTP %d: %.200s", path, status, body)
	}
	var st examState
	if err := json.Unmarshal(body, &st); err != nil {
		return nil, err
	}
	if st.Exam.ID == 0 {
		return nil, fmt.Errorf("%s is not a timed exam", seq)
	}
	return &st, nil
}

var questionsRe = regexp.MustCompile(`\s*\(\d+ Questions?\)$`)

// examSequence resolves an exam subsection by usage key, link, hex id or a unique part of its
// name. Exams are missing from the blocks API, so the course outline is searched instead.
func (c *Client) examSequence(ctx context.Context, courseID, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if m := usageKeyRe.FindString(ref); m != "" {
		ref = m
	}
	var raw struct {
		CourseBlocks struct {
			Blocks map[string]struct {
				Type string `json:"type"`
				Name string `json:"display_name"`
			} `json:"blocks"`
		} `json:"course_blocks"`
	}
	if err := c.API(ctx, "/api/course_home/outline/"+url.PathEscape(courseID), &raw); err != nil {
		return "", err
	}
	var exact, partial []string
	for id, b := range raw.CourseBlocks.Blocks {
		if b.Type != "sequential" {
			continue
		}
		switch {
		// The outline appends the number of questions to a name: «Промежуточный экзамен (20 Questions)».
		case id == ref || strings.HasSuffix(id, "@"+ref) || strings.EqualFold(questionsRe.ReplaceAllString(b.Name, ""), ref):
			exact = append(exact, id)
		case strings.Contains(strings.ToLower(b.Name), strings.ToLower(ref)):
			partial = append(partial, id)
		}
	}
	if len(exact) == 0 {
		exact = partial
	}
	switch len(exact) {
	case 1:
		return exact[0], nil
	case 0:
		return "", fmt.Errorf("no subsection matches %q", ref)
	default:
		var names []string
		for _, id := range exact {
			names = append(names, raw.CourseBlocks.Blocks[id].Name)
		}
		return "", fmt.Errorf("%q matches several subsections: %s", ref, strings.Join(names, "; "))
	}
}

// csrfToken returns the LMS CSRF token, asking the LMS for one when no cookie carries it.
// The caller holds c.mu.
func (c *Client) csrfToken(ctx context.Context) string {
	lms, _ := url.Parse(LMSURL)
	for _, ck := range c.client.Jar.Cookies(lms) {
		if ck.Name == "csrftoken" {
			return ck.Value
		}
	}
	body, status, _, err := c.fetch(ctx, LMSURL+"/csrf/api/v1/token")
	if err != nil || status != http.StatusOK {
		return ""
	}
	var t struct {
		Token string `json:"csrfToken"`
	}
	_ = json.Unmarshal(body, &t)
	return t.Token
}
