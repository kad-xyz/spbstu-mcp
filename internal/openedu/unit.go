package openedu

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type UnitLink struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// Unit is the readable content of one course page (an Open edX vertical).
type Unit struct {
	Name  string     `json:"name"`
	ID    string     `json:"id"`
	Text  string     `json:"text"`
	Links []UnitLink `json:"links,omitempty"`
}

type courseBlock struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Name     string   `json:"display_name"`
	Children []string `json:"children"`
}

// maxUnits bounds one call, so asking for a whole chapter cannot pull dozens of pages.
const maxUnits = 20

var usageKeyRe = regexp.MustCompile(`block-v1:[^/?#"\s]+`)

// Units returns the text and links of the pages under a course block. The block is a usage key
// (block-v1:…), a link containing one (an outline URL), the hex id at its end, or a part of a
// subsection or page name; subsections and chapters expand into their pages.
func (c *Client) Units(ctx context.Context, courseID, block string) ([]Unit, error) {
	pages, err := c.pages(ctx, courseID, block)
	if err != nil {
		return nil, err
	}
	out := make([]Unit, 0, len(pages))
	for _, pg := range pages {
		body, err := c.LMS(ctx, "/xblock/"+pg.ID)
		if err != nil {
			return nil, err
		}
		text, links := pageText(body)
		out = append(out, Unit{Name: pg.Name, ID: pg.ID, Text: text, Links: links})
	}
	return out, nil
}

// pages resolves a course block to the pages (verticals) under it, at most maxUnits of them.
func (c *Client) pages(ctx context.Context, courseID, block string) ([]courseBlock, error) {
	blocks, err := c.courseBlocks(ctx, courseID)
	if err != nil {
		return nil, err
	}
	b, err := findBlock(blocks, block)
	if err != nil {
		return nil, err
	}
	var pages []courseBlock
	var walk func(id string)
	walk = func(id string) {
		x, ok := blocks[id]
		if !ok {
			return
		}
		if x.Type == "vertical" {
			pages = append(pages, x)
			return
		}
		for _, ch := range x.Children {
			walk(ch)
		}
	}
	walk(b.ID)
	if len(pages) == 0 {
		return nil, fmt.Errorf("%q has no pages", b.Name)
	}
	if len(pages) > maxUnits {
		return nil, fmt.Errorf("%q has %d pages; ask for one subsection (at most %d pages)", b.Name, len(pages), maxUnits)
	}
	return pages, nil
}

func (c *Client) courseBlocks(ctx context.Context, courseID string) (map[string]courseBlock, error) {
	var me struct {
		Username string `json:"username"`
	}
	if err := c.API(ctx, "/api/user/v1/me", &me); err != nil {
		return nil, err
	}
	q := url.Values{
		"course_id":        {courseID},
		"username":         {me.Username},
		"depth":            {"all"},
		"requested_fields": {"children,display_name,type"},
	}
	var raw struct {
		Blocks map[string]courseBlock `json:"blocks"`
	}
	if err := c.API(ctx, "/api/courses/v2/blocks/?"+q.Encode(), &raw); err != nil {
		return nil, err
	}
	return raw.Blocks, nil
}

// findBlock resolves a usage key, a link with one, a hex id or a unique part of a name.
func findBlock(blocks map[string]courseBlock, ref string) (courseBlock, error) {
	ref = strings.TrimSpace(ref)
	if m := usageKeyRe.FindString(ref); m != "" {
		ref = m
		if u, err := url.PathUnescape(m); err == nil {
			ref = u
		}
	}
	if b, ok := blocks[ref]; ok {
		return b, nil
	}
	for id, b := range blocks {
		if strings.HasSuffix(id, "@"+ref) {
			return b, nil
		}
	}
	var hits []courseBlock
	low := strings.ToLower(ref)
	for _, b := range blocks {
		if (b.Type == "sequential" || b.Type == "vertical") && strings.Contains(strings.ToLower(b.Name), low) {
			hits = append(hits, b)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return courseBlock{}, fmt.Errorf("no subsection or page matches %q", ref)
	}
	names := make([]string, 0, len(hits))
	for _, h := range hits {
		names = append(names, h.Name)
	}
	if len(names) > 10 {
		names = append(names[:10], "…")
	}
	return courseBlock{}, fmt.Errorf("%q matches %d blocks, be more specific: %s", ref, len(hits), strings.Join(names, "; "))
}

var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true, "table": true, "section": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "pre": true, "ul": true, "ol": true,
}

// pageText turns a rendered xblock page into plain text and its links (videos and scripts skipped).
func pageText(page []byte) (string, []UnitLink) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return "", nil
	}
	base, _ := url.Parse(LMSURL + "/")
	var sb strings.Builder
	var links []UnitLink
	seen := map[string]bool{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "head", "template", "button":
				return
			}
			// Hidden helpers for screen readers, such as «skip to main content».
			for _, cl := range strings.Fields(attr(n, "class")) {
				if cl == "sr" || cl == "sr-only" || cl == "nav-skip" {
					return
				}
			}
			if blockTags[n.Data] {
				sb.WriteString("\n")
			}
			if n.Data == "li" {
				sb.WriteString("- ")
			}
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := attr(n, "href"); href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") {
				if u, err := base.Parse(href); err == nil && !seen[u.String()] {
					seen[u.String()] = true
					links = append(links, UnitLink{Text: collapse(nodeText(n)), URL: u.String()})
				}
			}
		}
	}
	walk(doc)
	return tidy(sb.String()), links
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
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return sb.String()
}

var spaceRe = regexp.MustCompile(`[ \t\r\f\v\x{a0}]+`)

func collapse(s string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// tidy collapses spaces inside lines and drops empty lines.
func tidy(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = collapse(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
