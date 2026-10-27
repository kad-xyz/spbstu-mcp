package openedu

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestThreadID(t *testing.T) {
	for in, want := range map[string]string{
		"https://courses.openedu.ru/courses/course-v1:spbstu+WEBPYT+fall_2026/discussion/forum/course/threads/6a707596d7a8be0026ce3ff5": "6a707596d7a8be0026ce3ff5",
		"6a707596d7a8be0026ce3ff5": "6a707596d7a8be0026ce3ff5",
	} {
		if got, err := threadID(in); err != nil || got != want {
			t.Errorf("%q → %q, %v", in, got, err)
		}
	}
	if _, err := threadID("course"); err == nil {
		t.Error("want an error without an id")
	}
}

type forumFake map[string]string

func (f forumFake) RoundTrip(r *http.Request) (*http.Response, error) {
	body, ok := f[r.URL.RequestURI()]
	status := http.StatusOK
	if !ok {
		status, body = http.StatusNotFound, "{}"
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func TestForumThreadReplies(t *testing.T) {
	const id = "6a707596d7a8be0026ce3ff5"
	c := &Client{client: &http.Client{Transport: forumFake{
		"/api/discussion/v1/threads/" + id + "/": `{"title":"Доступ","author":"staff","type":"discussion"}`,
		"/api/discussion/v1/comments/?page_size=100&thread_id=" + id: `{"results":[
			{"id":"c1","author":"student","raw_body":"Откройте тесты","child_count":1},
			{"id":"c2","author":"other","raw_body":"И мне"}],"pagination":{}}`,
		"/api/discussion/v1/comments/c1/?page_size=100": `{"results":[
			{"id":"r1","author":"staff","author_label":"Staff","raw_body":"Открыто"}],"pagination":{}}`,
	}}}
	th, err := c.ForumThread(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Comments) != 2 || len(th.Comments[0].Replies) != 1 || th.Comments[0].Replies[0].Body != "Открыто" {
		t.Fatalf("replies not read: %+v", th.Comments)
	}
	if th.Comments[1].Replies != nil {
		t.Errorf("a comment without replies got some: %+v", th.Comments[1])
	}
}
