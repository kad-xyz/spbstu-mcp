// Package answers looks up test questions in the public answer bases of SPbPU students:
// polyvsp.ru (a table of questions and answers per course) and polyphis.ru (paged lists with a
// search by question). Pages are cached on disk; polyvsp.ru, which is often down, falls back to
// its copy on web.archive.org.
package answers

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

const (
	VSPURL  = "https://polyvsp.ru"
	PhisURL = "https://polyphis.ru"
	// BigBase is the polyvsp.ru page that merges answers from many courses, searched when no subject is given.
	BigBase = VSPURL + "/largedata/"
)

// Subject is a course page of an answer base.
type Subject struct {
	Site string `json:"site"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Entry is one question with its answer as the base gives it.
type Entry struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Source   string `json:"source"`
}

// Match is an entry found for a question; Score is the share of common words, 0..1.
type Match struct {
	Entry
	Score float64 `json:"score"`
}

// Result holds the best matches of one question. Found means the best match is close enough
// to take its answer.
type Result struct {
	Question string  `json:"question"`
	Found    bool    `json:"found"`
	Matches  []Match `json:"matches,omitempty"`
}

const (
	// FoundScore is the score from which a match counts as the same question.
	FoundScore = 0.8
	minScore   = 0.45
	maxMatches = 3
	// phisPages bounds the result pages read per polyphis.ru search.
	phisPages = 3
)

type Client struct {
	CacheDir string
	TTL      time.Duration
	HTTP     *http.Client
}

// Default caches pages for a week in ~/.config/spbstu-mcp/answers.
func Default() *Client {
	home, _ := os.UserHomeDir()
	return &Client{CacheDir: filepath.Join(home, ".config", "spbstu-mcp", "answers"), TTL: 7 * 24 * time.Hour}
}

// Subjects lists the course pages of both bases whose name contains query (all when empty).
func (c *Client) Subjects(ctx context.Context, query string) ([]Subject, error) {
	var out []Subject
	var errs []error
	if body, err := c.fetch(ctx, VSPURL+"/vibpredmet/"); err != nil {
		errs = append(errs, err)
	} else {
		out = append(out, parseVSPSubjects(body)...)
	}
	if body, err := c.fetch(ctx, PhisURL+"/"); err != nil {
		errs = append(errs, err)
	} else {
		out = append(out, parsePhisSubjects(body)...)
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	q := normalize(query)
	if q == "" {
		return out, nil
	}
	var hit []Subject
	for _, s := range out {
		if strings.Contains(normalize(s.Name), q) || strings.Contains(s.URL, strings.ToLower(query)) {
			hit = append(hit, s)
		}
	}
	return hit, nil
}

// Search looks the questions up on the given subject pages (links from Subjects; the big
// polyvsp.ru base when none). A page that cannot be read is reported in the notes.
func (c *Client) Search(ctx context.Context, questions, subjects []string) ([]Result, []string, error) {
	if len(questions) == 0 {
		return nil, nil, errors.New("no questions given")
	}
	if len(subjects) == 0 {
		subjects = []string{BigBase}
	}
	var notes []string
	var pool []Entry // entries of polyvsp.ru pages, read once for all questions
	var phis []string
	for _, s := range subjects {
		u, err := subjectURL(s)
		if err != nil {
			notes = append(notes, err.Error())
			continue
		}
		if strings.HasPrefix(u, PhisURL) {
			phis = append(phis, u)
			continue
		}
		body, err := c.fetch(ctx, u)
		if err != nil {
			notes = append(notes, err.Error())
			continue
		}
		entries := parseVSP(body, u)
		if len(entries) == 0 {
			notes = append(notes, u+": no question table on the page")
		}
		pool = append(pool, entries...)
	}
	out := make([]Result, 0, len(questions))
	for _, q := range questions {
		cands := pool
		for _, u := range phis {
			found, err := c.searchPhis(ctx, u, q)
			if err != nil {
				notes = append(notes, err.Error())
			}
			cands = append(cands, found...)
		}
		out = append(out, best(q, cands))
	}
	return out, dedupe(notes), nil
}

// best ranks the candidates for a question and keeps the close ones, one per distinct answer.
func best(q string, cands []Entry) Result {
	qt := tokens(q)
	var ms []Match
	for _, e := range cands {
		if s := score(qt, q, e.Question); s >= minScore {
			ms = append(ms, Match{Entry: e, Score: s})
		}
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Score > ms[j].Score })
	seen := map[string]bool{}
	var keep []Match
	for _, m := range ms {
		k := normalize(m.Question) + "\x00" + normalize(m.Answer)
		if seen[k] {
			continue
		}
		seen[k] = true
		m.Score = float64(int(m.Score*100+0.5)) / 100
		keep = append(keep, m)
		if len(keep) == maxMatches {
			break
		}
	}
	return Result{Question: q, Found: len(keep) > 0 && keep[0].Score >= FoundScore, Matches: keep}
}

// score is the Dice similarity of the word stems; a question that contains the other whole
// (one of them cut short or with an instruction added) counts as close.
func score(qt map[string]bool, q, cand string) float64 {
	ct := tokens(cand)
	if len(qt) == 0 || len(ct) == 0 {
		return 0
	}
	common := 0
	for t := range qt {
		if ct[t] {
			common++
		}
	}
	s := 2 * float64(common) / float64(len(qt)+len(ct))
	nq, nc := normalize(q), normalize(cand)
	short, long := nq, nc
	if len(short) > len(long) {
		short, long = long, short
	}
	if len(strings.Fields(short)) >= 4 && strings.Contains(long, short) && s < 0.9 {
		s = 0.9
	}
	return s
}

// tokens returns the stems (first five letters) of the words of s.
func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(normalize(s)) {
		r := []rune(w)
		if len(r) < 2 && !unicode.IsDigit(r[0]) {
			continue
		}
		if len(r) > 5 {
			r = r[:5]
		}
		out[string(r)] = true
	}
	return out
}

// normalize lowercases s, folds ё into е and leaves only letters and digits separated by spaces.
func normalize(s string) string {
	var sb strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if r == 'ё' {
			r = 'е'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
			space = false
		} else if !space {
			sb.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(sb.String())
}

// subjectURL turns a link or a bare page name (bgd, polyphis.ru/bzd) into a page URL of a base.
func subjectURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		if strings.HasPrefix(s, "polyvsp.ru") || strings.HasPrefix(s, "polyphis.ru") {
			s = "https://" + s
		} else {
			s = VSPURL + "/" + strings.Trim(s, "/") + "/"
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%s: not a link", s)
	}
	switch u.Host {
	case "polyvsp.ru", "www.polyvsp.ru":
		return VSPURL + "/" + strings.Trim(u.Path, "/") + "/", nil
	case "polyphis.ru", "www.polyphis.ru":
		return PhisURL + "/" + strings.Trim(u.Path, "/"), nil
	}
	return "", fmt.Errorf("%s: only polyvsp.ru and polyphis.ru pages are answer bases", s)
}

// searchPhis runs the polyphis.ru search for a few fragments of the question (it matches a
// fragment exactly, so the whole question rarely works) and collects the entries it returns.
func (c *Client) searchPhis(ctx context.Context, page, q string) ([]Entry, error) {
	var out []Entry
	var lastErr error
	for _, f := range fragments(q) {
		for p := 1; p <= phisPages; p++ {
			v := url.Values{"search": {f}}
			if p > 1 {
				v.Set("page", strconv.Itoa(p))
			}
			body, err := c.fetch(ctx, page+"?"+v.Encode())
			if err != nil {
				lastErr = err
				break
			}
			entries, pages := parsePhis(body, page)
			out = append(out, entries...)
			if p >= pages {
				break
			}
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

var edgePunct = func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// fragments picks search strings from a question: its first three words, and the two
// longest words each with the word after it.
func fragments(q string) []string {
	words := strings.Fields(q)
	for i, w := range words {
		words[i] = strings.TrimFunc(w, edgePunct)
	}
	var clean []string
	for _, w := range words {
		if w != "" {
			clean = append(clean, w)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(f string) {
		k := strings.ToLower(f)
		if f != "" && !seen[k] {
			seen[k] = true
			out = append(out, f)
		}
	}
	add(strings.Join(clean[:min(3, len(clean))], " "))
	idx := make([]int, len(clean))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return len([]rune(clean[idx[a]])) > len([]rune(clean[idx[b]]))
	})
	for _, i := range idx[:min(2, len(idx))] {
		if i+1 < len(clean) {
			add(clean[i] + " " + clean[i+1])
		} else {
			add(clean[i])
		}
	}
	return out
}

var pagesRe = regexp.MustCompile(`Страница\s+\d+\s+из\s+(\d+)`)

// parsePhis reads a polyphis.ru list page: a question in <strong>, its answer in the next <p>.
// It also returns the number of result pages.
func parsePhis(body []byte, src string) ([]Entry, int) {
	pages := 1
	if m := pagesRe.FindSubmatch(body); m != nil {
		pages, _ = strconv.Atoi(string(m[1]))
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, pages
	}
	var out []Entry
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "strong" && n.Parent != nil && hasClass(n.Parent, "item2") {
			q := collapse(nodeText(n))
			for s := n.NextSibling; s != nil; s = s.NextSibling {
				if s.Type != html.ElementNode {
					continue
				}
				if s.Data == "p" {
					if a := collapse(nodeText(s)); q != "" && a != "" {
						out = append(out, Entry{Question: q, Answer: a, Source: src})
					}
				}
				break
			}
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return out, pages
}

// parseVSP reads the question tables of a polyvsp.ru page (TablePress: column-1 the question,
// column-2 the answer; lines of a cell are joined with "; ").
func parseVSP(body []byte, src string) []Entry {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	var out []Entry
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var q, a string
			for td := n.FirstChild; td != nil; td = td.NextSibling {
				if td.Type != html.ElementNode || td.Data != "td" {
					continue
				}
				switch {
				case hasClass(td, "column-1"):
					q = cellText(td)
				case hasClass(td, "column-2"):
					a = cellText(td)
				}
			}
			if q != "" && a != "" {
				out = append(out, Entry{Question: q, Answer: a, Source: src})
			}
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return out
}

// skipVSP are the links of the polyvsp.ru course list that are not courses.
var skipVSP = regexp.MustCompile(`^/(|vibpredmet|feed|comments|author|wp-[a-z-]+|\d{4}|otzyvy|vakansii|zakaz)(/|$)`)

func parseVSPSubjects(body []byte) []Subject {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	var out []Subject
	seen := map[string]bool{}
	var walk func(n *html.Node, inPost bool)
	walk = func(n *html.Node, inPost bool) {
		if n.Type == html.ElementNode {
			if hasClass(n, "post-text") {
				inPost = true
			}
			if inPost && n.Data == "a" {
				u, err := url.Parse(attr(n, "href"))
				name := collapse(nodeText(n))
				if err == nil && strings.TrimPrefix(u.Host, "www.") == "polyvsp.ru" && name != "" && !skipVSP.MatchString(u.Path) &&
					strings.Count(strings.Trim(u.Path, "/"), "/") == 0 {
					link := VSPURL + "/" + strings.Trim(u.Path, "/") + "/"
					if !seen[link] {
						seen[link] = true
						out = append(out, Subject{Site: "polyvsp.ru", Name: name, URL: link})
					}
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch, inPost)
		}
	}
	walk(doc, false)
	return out
}

// skipPhis are the polyphis.ru menu links without a question list: ordering, accounts, and the
// physics and maths calculators.
var skipPhis = regexp.MustCompile(`^/(zakaz|about|accounts|fiz\d|math\d|calc)`)

func parsePhisSubjects(body []byte) []Subject {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	var out []Subject
	seen := map[string]bool{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" && hasClass(n, "dropdown-item") {
			href, name := attr(n, "href"), collapse(nodeText(n))
			if strings.HasPrefix(href, "/") && len(href) > 1 && !skipPhis.MatchString(href) && name != "" && !seen[href] {
				seen[href] = true
				out = append(out, Subject{Site: "polyphis.ru", Name: name, URL: PhisURL + href})
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return out
}

// fetch returns a page from the cache while it is fresh, else downloads it; polyvsp.ru pages
// fall back to web.archive.org, and a stale cached copy is better than none.
func (c *Client) fetch(ctx context.Context, link string) ([]byte, error) {
	path := filepath.Join(c.CacheDir, key(link)+".html")
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) < c.TTL {
		if b, err := os.ReadFile(path); err == nil {
			return b, nil
		}
	}
	body, err := c.get(ctx, link, 30*time.Second)
	if err != nil && strings.HasPrefix(link, VSPURL) {
		var aerr error
		if body, aerr = c.get(ctx, "https://web.archive.org/web/2099id_/"+link, 90*time.Second); aerr != nil {
			err = fmt.Errorf("%w; web.archive.org: %v", err, aerr)
		} else {
			err = nil
		}
	}
	if err != nil {
		if b, rerr := os.ReadFile(path); rerr == nil {
			return b, nil
		}
		return nil, err
	}
	if os.MkdirAll(c.CacheDir, 0o700) == nil {
		_ = os.WriteFile(path, body, 0o600)
	}
	return body, nil
}

func (c *Client) get(ctx context.Context, link string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", link, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", link, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func key(link string) string {
	h := sha1.Sum([]byte(link))
	return hex.EncodeToString(h[:])
}

func dedupe(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func nodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			sb.WriteString(n.Data)
		case n.Type == html.ElementNode && n.Data == "br":
			sb.WriteString("\n")
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return sb.String()
}

// cellText joins the lines of a table cell with "; ".
func cellText(n *html.Node) string {
	var lines []string
	for _, l := range strings.Split(nodeText(n), "\n") {
		if l = collapse(l); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "; ")
}

func collapse(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, " ", " ")), " ")
}
