package openedu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
)

// Thread is a discussion thread of a course forum with its responses.
type Thread struct {
	ID       string    `json:"thread_id"`
	Title    string    `json:"title"`
	Author   string    `json:"author"`
	Created  string    `json:"created"`
	Body     string    `json:"body"`
	Closed   bool      `json:"closed,omitempty"`
	Comments []Comment `json:"comments"`
}

// Comment is a response in a thread with the replies to it.
type Comment struct {
	ID      string    `json:"comment_id"`
	Author  string    `json:"author"`
	Label   string    `json:"author_label,omitempty"`
	Created string    `json:"created"`
	Body    string    `json:"body"`
	Replies []Comment `json:"replies,omitempty"`
}

type rawComment struct {
	ID         string `json:"id"`
	Author     string `json:"author"`
	Label      string `json:"author_label"`
	Created    string `json:"created_at"`
	Body       string `json:"raw_body"`
	ChildCount int    `json:"child_count"`
}

func (r rawComment) comment() Comment {
	return Comment{ID: r.ID, Author: r.Author, Label: r.Label, Created: localTime(r.Created), Body: r.Body}
}

// comments reads every page of a comments listing, then the replies of the comments that have any.
func (c *Client) comments(ctx context.Context, path string) ([]Comment, error) {
	var out []Comment
	for path != "" {
		var page struct {
			Results    []rawComment `json:"results"`
			Pagination struct {
				Next string `json:"next"`
			} `json:"pagination"`
		}
		if err := c.API(ctx, path, &page); err != nil {
			return nil, err
		}
		for _, r := range page.Results {
			cm := r.comment()
			if r.ChildCount > 0 {
				replies, err := c.comments(ctx, "/api/discussion/v1/comments/"+url.PathEscape(r.ID)+"/?page_size=100")
				if err != nil {
					return nil, err
				}
				cm.Replies = replies
			}
			out = append(out, cm)
		}
		path = ""
		if u, err := url.Parse(page.Pagination.Next); err == nil && page.Pagination.Next != "" {
			path = u.RequestURI()
		}
	}
	return out, nil
}

var threadIDRe = regexp.MustCompile(`[0-9a-f]{24}`)

// threadID takes a thread id or a forum link that ends in one.
func threadID(ref string) (string, error) {
	ids := threadIDRe.FindAllString(ref, -1)
	if len(ids) == 0 {
		return "", fmt.Errorf("no thread id in %q", ref)
	}
	return ids[len(ids)-1], nil
}

// ForumThread reads a forum thread, its responses and the replies to them.
func (c *Client) ForumThread(ctx context.Context, ref string) (*Thread, error) {
	id, err := threadID(ref)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Title   string `json:"title"`
		Author  string `json:"author"`
		Created string `json:"created_at"`
		Body    string `json:"raw_body"`
		Type    string `json:"type"`
		Closed  bool   `json:"closed"`
	}
	if err := c.API(ctx, "/api/discussion/v1/threads/"+id+"/", &raw); err != nil {
		return nil, err
	}
	t := &Thread{ID: id, Title: raw.Title, Author: raw.Author, Created: localTime(raw.Created), Body: raw.Body, Closed: raw.Closed}
	// A question thread lists endorsed answers apart from the rest.
	queries := []string{""}
	if raw.Type == "question" {
		queries = []string{"&endorsed=true", "&endorsed=false"}
	}
	for _, q := range queries {
		cs, err := c.comments(ctx, "/api/discussion/v1/comments/?page_size=100&thread_id="+url.QueryEscape(id)+q)
		if err != nil {
			return nil, err
		}
		t.Comments = append(t.Comments, cs...)
	}
	return t, nil
}

// ForumReply posts a response to a forum thread and returns it as the forum stored it.
func (c *Client) ForumReply(ctx context.Context, ref, body string) (*Comment, error) {
	id, err := threadID(ref)
	if err != nil {
		return nil, err
	}
	b, err := c.jsonLMS(ctx, "/api/discussion/v1/comments/", map[string]string{"thread_id": id, "raw_body": body})
	if err != nil {
		return nil, fmt.Errorf("reply not posted: %w", err)
	}
	var r rawComment
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if r.ID == "" {
		return nil, fmt.Errorf("the forum answered without a comment: %.200s", b)
	}
	cm := r.comment()
	return &cm, nil
}
