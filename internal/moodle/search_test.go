package moodle

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeWS map[string]string

func (f fakeWS) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	body, ok := f[r.PostForm.Get("wsfunction")]
	if !ok {
		body = `{"exception":"x","errorcode":"invalidrecord","message":"unexpected call"}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

func TestSearchGivesInstanceID(t *testing.T) {
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = fakeWS{
		"core_enrol_get_users_courses": `[{"id":154,"fullname":"Практики и НИР"}]`,
		"core_course_get_contents": `[{"name":"Учебная практика - 2 курс","modules":[
			{"id":220692,"instance":29978,"name":"Ознакомительная практика (2026, заочники)","modname":"forum","uservisible":true},
			{"id":94526,"instance":12009,"name":"Презентация","modname":"resource","uservisible":true}]}]`,
	}
	t.Cleanup(func() { http.DefaultClient.Transport = old })

	c := &Client{token: "t", userID: 1}
	hits, err := c.Search(context.Background(), "ознакомительная практика", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].CMID != 220692 || hits[0].InstanceID != 29978 {
		t.Fatalf("hits = %+v, want the forum with cmid 220692 and instance_id 29978", hits)
	}
}
