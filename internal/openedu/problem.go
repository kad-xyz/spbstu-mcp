package openedu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Input is one answer field of a problem.
type Input struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"` // radio (one choice), checkbox (several), select (one option), text
	Prompt  string   `json:"prompt,omitempty"`
	Choices []Choice `json:"choices,omitempty"`
	Answer  []string `json:"answer,omitempty"`
	Status  string   `json:"status,omitempty"`
}

// Problem is a question (an Open edX capa problem) with the user's current answers.
type Problem struct {
	ID       string  `json:"problem_id"`
	Unit     string  `json:"unit,omitempty"`
	Title    string  `json:"title"`
	Score    string  `json:"score"`
	Attempts string  `json:"attempts,omitempty"`
	Graded   bool    `json:"graded"`
	Inputs   []Input `json:"inputs"`
	Notice   string  `json:"notice,omitempty"`
	url      string
}

// Problems returns the questions on the pages under a course block, resolved as in Units.
func (c *Client) Problems(ctx context.Context, courseID, block string) ([]Problem, error) {
	pages, err := c.pages(ctx, courseID, block)
	if err != nil {
		return nil, err
	}
	var out []Problem
	for _, pg := range pages {
		body, err := c.LMS(ctx, "/xblock/"+pg.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range parseProblems(body) {
			p.Unit = pg.Name
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no questions on %q (timed exams open only in the browser)", block)
	}
	return out, nil
}

// Answer saves answers to a problem, or with submit checks them, which uses up an attempt.
// Answers map an input name to its value: one for radio, select and text, any for checkbox.
func (c *Client) Answer(ctx context.Context, courseID, problem string, answers map[string][]string, submit bool) (*Problem, error) {
	blocks, err := c.courseBlocks(ctx, courseID)
	if err != nil {
		return nil, err
	}
	b, err := findProblem(blocks, problem)
	if err != nil {
		// Exam questions are missing from the blocks API, so a full usage key is taken as is.
		key := usageKeyRe.FindString(problem)
		if !strings.Contains(key, "type@problem+") {
			return nil, err
		}
		b = courseBlock{ID: key, Type: "problem"}
	}
	body, err := c.LMS(ctx, "/xblock/"+b.ID)
	if err != nil {
		return nil, err
	}
	ps := parseProblems(body)
	if len(ps) != 1 {
		return nil, fmt.Errorf("problem %s did not render", b.ID)
	}
	form, err := answerForm(ps[0].Inputs, answers)
	if err != nil {
		return nil, err
	}
	handler := strings.TrimPrefix(ps[0].url, LMSURL)
	action := "/problem_save"
	if submit {
		action = "/problem_check"
		// A wrong answer locks the problem until it is reset; resetting spends no attempt.
		if needsReset(ps[0].Inputs) {
			if err := c.resetProblem(ctx, handler); err != nil {
				return nil, err
			}
		}
	}
	raw, err := c.PostLMS(ctx, handler+action, form)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Success  any    `json:"success"`
		Msg      string `json:"msg"`
		Contents string `json:"contents"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("unexpected answer response: %.200s", raw)
	}
	// problem_check reports a refusal (no attempts left, past due) in success itself.
	// Exams hide the result and report "submitted".
	if s, ok := resp.Success.(string); ok && submit && s != "correct" && s != "incorrect" && s != "partially-correct" && s != "submitted" {
		return nil, fmt.Errorf("not accepted: %s", s)
	}
	if ok, isBool := resp.Success.(bool); isBool && !ok {
		return nil, fmt.Errorf("not saved: %s", resp.Msg)
	}
	// Re-read the problem so the result shows the stored answers, score and attempts.
	if body, err = c.LMS(ctx, "/xblock/"+b.ID); err != nil {
		return nil, err
	}
	if ps = parseProblems(body); len(ps) != 1 {
		return nil, fmt.Errorf("problem %s did not render", b.ID)
	}
	p := ps[0]
	if resp.Msg != "" {
		p.Notice = textOf(resp.Msg)
	}
	return &p, nil
}

func needsReset(inputs []Input) bool {
	for _, in := range inputs {
		if in.Status == "incorrect" || in.Status == "partially-correct" {
			return true
		}
	}
	return false
}

func (c *Client) resetProblem(ctx context.Context, handler string) error {
	raw, err := c.PostLMS(ctx, handler+"/problem_reset", url.Values{})
	if err != nil {
		return err
	}
	var resp struct {
		Success any    `json:"success"`
		Msg     string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("unexpected reset response: %.200s", raw)
	}
	if ok, isBool := resp.Success.(bool); isBool && !ok {
		return fmt.Errorf("not reset: %s", textOf(resp.Msg))
	}
	return nil
}

func findProblem(blocks map[string]courseBlock, ref string) (courseBlock, error) {
	ref = strings.TrimSpace(ref)
	if m := usageKeyRe.FindString(ref); m != "" {
		ref = m
	}
	for id, b := range blocks {
		if b.Type == "problem" && (id == ref || strings.HasSuffix(id, "@"+ref)) {
			return b, nil
		}
	}
	return courseBlock{}, fmt.Errorf("no problem %q in the course; take problem_id from openedu_problems", ref)
}

// answerForm checks answers against a problem's inputs and encodes them as the problem form.
func answerForm(inputs []Input, answers map[string][]string) (url.Values, error) {
	byName := map[string]Input{}
	for _, in := range inputs {
		byName[in.Name] = in
	}
	form := url.Values{}
	for name, vals := range answers {
		in, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("the problem has no input %q", name)
		}
		if in.Type != "checkbox" && len(vals) != 1 {
			return nil, fmt.Errorf("%s takes exactly one value", name)
		}
		if len(in.Choices) > 0 {
			for _, v := range vals {
				if !hasChoice(in.Choices, v) {
					return nil, fmt.Errorf("%s has no choice %q", name, v)
				}
			}
		}
		key := name
		if in.Type == "checkbox" {
			key += "[]"
		}
		form[key] = vals
	}
	// The LMS takes the whole form, so unchanged inputs keep their stored answers.
	for _, in := range inputs {
		if _, ok := answers[in.Name]; ok || len(in.Answer) == 0 {
			continue
		}
		key := in.Name
		if in.Type == "checkbox" {
			key += "[]"
		}
		form[key] = in.Answer
	}
	return form, nil
}

func hasChoice(choices []Choice, v string) bool {
	for _, ch := range choices {
		if ch.Value == v {
			return true
		}
	}
	return false
}

var attemptsRe = regexp.MustCompile(`(\d+)\s+из\s+(\d+)`)

// parseProblems finds the problems of a rendered page. Each keeps its markup in the
// data-content attribute of its wrapper, next to its score and handler URL.
func parseProblems(page []byte) []Problem {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil
	}
	var out []Problem
	var graded bool
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && attr(n, "data-block-type") == "problem" {
			graded = attr(n, "data-graded") == "True"
		}
		if n.Type == html.ElementNode && hasClass(n, "problems-wrapper") {
			p := Problem{
				ID:     attr(n, "data-problem-id"),
				Score:  attr(n, "data-problem-score") + "/" + attr(n, "data-problem-total-possible"),
				Graded: graded,
				url:    LMSURL + attr(n, "data-url"),
			}
			parseContent(&p, attr(n, "data-content"))
			out = append(out, p)
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return out
}

func parseContent(p *Problem, content string) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return
	}
	var prompt strings.Builder
	var cur *Input
	inTable := 0
	add := func(in Input) {
		in.Prompt = tidy(prompt.String())
		prompt.Reset()
		p.Inputs = append(p.Inputs, in)
		cur = &p.Inputs[len(p.Inputs)-1]
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			if inTable > 0 {
				prompt.WriteString(strings.ReplaceAll(n.Data, "\n", " "))
			} else {
				prompt.WriteString(n.Data)
			}
			return
		}
		if n.Type != html.ElementNode {
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				walk(ch)
			}
			return
		}
		switch {
		case n.Data == "script" || n.Data == "style" || n.Data == "button" || hasClass(n, "sr"):
			return
		case n.Data == "h3" && hasClass(n, "problem-header"):
			p.Title = collapse(nodeText(n))
			return
		case hasClass(n, "submission-feedback"):
			if m := attemptsRe.FindStringSubmatch(nodeText(n)); m != nil {
				p.Attempts = m[1] + "/" + m[2]
			}
			return
		case hasClass(n, "notification") || hasClass(n, "solution-span"):
			return
		case n.Data == "span" && hasClass(n, "status") && cur != nil:
			cur.Status = statusOf(n)
			return
		case n.Data == "select":
			in := Input{Name: attr(n, "name"), Type: "select"}
			for o := range n.Descendants() {
				if o.Type != html.ElementNode || o.Data != "option" || strings.HasSuffix(attr(o, "value"), "_dummy_default") {
					continue
				}
				in.Choices = append(in.Choices, Choice{Value: attr(o, "value"), Label: collapse(nodeText(o))})
				if hasAttr(o, "selected") {
					in.Answer = []string{attr(o, "value")}
				}
			}
			add(in)
			return
		case n.Data == "input" && attr(n, "type") == "text":
			in := Input{Name: attr(n, "name"), Type: "text"}
			if v := attr(n, "value"); v != "" {
				in.Answer = []string{v}
			}
			add(in)
			return
		case n.Data == "fieldset":
			// A choice group: the legend is the question, the inputs its choices.
			var in Input
			labels := map[string]string{}
			for x := range n.Descendants() {
				if x.Type != html.ElementNode {
					continue
				}
				if x.Data == "legend" {
					prompt.WriteString("\n" + nodeText(x))
				}
				if x.Data == "input" && (attr(x, "type") == "radio" || attr(x, "type") == "checkbox") {
					in.Name, in.Type = strings.TrimSuffix(attr(x, "name"), "[]"), attr(x, "type")
					in.Choices = append(in.Choices, Choice{Value: attr(x, "value"), Label: attr(x, "id")})
					if hasAttr(x, "checked") {
						in.Answer = append(in.Answer, attr(x, "value"))
					}
				}
				if x.Data == "label" && attr(x, "for") != "" {
					// A choice may be a picture, so its label keeps the image links.
					label := collapse(nodeText(x))
					for img := range x.Descendants() {
						if img.Type == html.ElementNode && img.Data == "img" && attr(img, "src") != "" {
							label = strings.TrimSpace(label + " [изображение: " + absURL(attr(img, "src")) + "]")
						}
					}
					labels[attr(x, "for")] = label
				}
			}
			for i := range in.Choices {
				in.Choices[i].Label = labels[in.Choices[i].Label]
			}
			add(in)
			return
		}
		switch {
		case n.Data == "img":
			// Pictures (diagrams, tables as images) are part of the question.
			if src := attr(n, "src"); src != "" {
				prompt.WriteString(" [изображение: " + absURL(src) + "] ")
			}
		case n.Data == "table":
			// Table rows stay on one line, cells separated by bars.
			prompt.WriteString("\n")
			inTable++
			defer func() { inTable-- }()
		case n.Data == "td" || n.Data == "th":
			prompt.WriteString(" | ")
		case blockTags[n.Data] && inTable == 0:
			prompt.WriteString("\n")
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
		if n.Data == "tr" {
			prompt.WriteString(" |\n")
		}
	}
	walk(doc)
}

// absURL resolves a link of the course markup against the LMS.
func absURL(src string) string {
	if u, err := url.Parse(LMSURL + "/"); err == nil {
		if abs, err := u.Parse(src); err == nil {
			return abs.String()
		}
	}
	return src
}

// statusOf reads the state of an answer from the class of its status span.
func statusOf(n *html.Node) string {
	for _, cl := range strings.Fields(attr(n, "class")) {
		switch cl {
		case "correct", "incorrect", "partially-correct", "unanswered", "unsubmitted", "submitted":
			return cl
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, cl := range strings.Fields(attr(n, "class")) {
		if cl == class {
			return true
		}
	}
	return false
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func textOf(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}
	return collapse(nodeText(doc))
}

// PostLMS posts a form to an LMS handler the way the courseware page does, with the session's
// CSRF token, and returns the response body.
func (c *Client) PostLMS(ctx context.Context, path string, form url.Values) ([]byte, error) {
	return c.formLMS(ctx, http.MethodPost, path, form)
}

// formLMS sends a form to the LMS with the given method, as PostLMS does.
func (c *Client) formLMS(ctx context.Context, method, path string, form url.Values) ([]byte, error) {
	return c.sendLMS(ctx, method, path, "application/x-www-form-urlencoded; charset=UTF-8", form.Encode())
}

// jsonLMS posts a JSON body to an LMS API, as formLMS does a form.
func (c *Client) jsonLMS(ctx context.Context, path string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.sendLMS(ctx, http.MethodPost, path, "application/json", string(b))
}

func (c *Client) sendLMS(ctx context.Context, method, path, contentType, body string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	csrf := c.csrfToken(ctx)
	req, err := http.NewRequestWithContext(ctx, method, LMSURL+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-CSRFToken", csrf)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("USE-JWT-COOKIE", "true")
	req.Header.Set("Referer", LMSURL+"/")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("courses.openedu.ru%s: HTTP %d: %.200s", path, resp.StatusCode, buf.Bytes())
	}
	return buf.Bytes(), nil
}
