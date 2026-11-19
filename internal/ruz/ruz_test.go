package ruz

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeRUZ answers each request with the next body of its list, and counts the requests.
type fakeRUZ struct {
	bodies []string
	paths  []string
}

func (f *fakeRUZ) RoundTrip(r *http.Request) (*http.Response, error) {
	f.paths = append(f.paths, r.URL.Path+"?"+r.URL.RawQuery)
	body := f.bodies[0]
	if len(f.bodies) > 1 {
		f.bodies = f.bodies[1:]
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

func useFake(t *testing.T, bodies ...string) *fakeRUZ {
	t.Helper()
	f := &fakeRUZ{bodies: bodies}
	old, oldDelay := http.DefaultClient.Transport, retryDelay
	http.DefaultClient.Transport, retryDelay = f, 0
	t.Cleanup(func() { http.DefaultClient.Transport, retryDelay = old, oldDelay })
	return f
}

const cacheError = `{"error": true, "text": "Ошибка получения данных 'Занятия аудитории' с кэша"}`

const roomWeek = `{"week":{"date_start":"2026.11.16","date_end":"2026.11.22","is_odd":false},
 "room":{"id":967,"name":"Дистанционно","building":{"id":33,"name":"DL"}},
 "days":[{"weekday":4,"date":"2026-11-19","lessons":[
  {"subject":"Моделирование информационных систем","time_start":"10:00","time_end":"11:40","additional_info":"Поток",
   "typeObj":{"name":"Комиссия. Экзамен"},"groups":[{"name":"з5130902/40201"}],
   "teachers":[{"full_name":"Ефремов Артем Александрович"}],
   "auditories":[{"name":"Дистанционно","building":{"name":"DL"}}],
   "webinar_url":"https://my.mts-link.ru/j/SPbPU/7330287016","lms_url":""},
  {"subject":"Базы данных","time_start":"12:00","time_end":"13:40","typeObj":{"name":"Лекции"},
   "groups":[{"name":"з5130902/30201"}],"teachers":null,"auditories":[],"webinar_url":"","lms_url":""}]}]}`

func TestRoomWeekRetriesCacheError(t *testing.T) {
	f := useFake(t, cacheError, roomWeek)
	w, err := RoomWeek(context.Background(), 33, 967, time.Date(2026, 11, 19, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.paths) != 2 || f.paths[1] != "/api/v1/ruz/buildings/33/rooms/967/scheduler?date=2026-11-19" {
		t.Fatalf("requests = %v, want a retry of the room scheduler", f.paths)
	}
	if w.Of != "DL, ауд. Дистанционно" || len(w.Lessons) != 2 {
		t.Fatalf("week = %+v", w)
	}
	if l := w.Lessons[0]; l.Groups[0] != "з5130902/40201" || l.Webinar == "" || l.Weekday != "Чт" {
		t.Errorf("lesson = %+v, want its groups, webinar link and weekday", l)
	}
}

func TestCacheErrorIsNotAnEmptyWeek(t *testing.T) {
	f := useFake(t, cacheError)
	_, err := GroupWeek(context.Background(), 45851, time.Now())
	if err == nil || !strings.Contains(err.Error(), "с кэша") {
		t.Fatalf("err = %v, want RUZ's own error text", err)
	}
	if len(f.paths) != attempts {
		t.Errorf("made %d requests, want %d", len(f.paths), attempts)
	}
}

func TestFilter(t *testing.T) {
	useFake(t, roomWeek)
	w, err := RoomWeek(context.Background(), 33, 967, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	w.Filter("КОМИССИЯ ефремов")
	if len(w.Lessons) != 1 || w.Lessons[0].Subject != "Моделирование информационных систем" {
		t.Errorf("lessons = %+v, want only the commission", w.Lessons)
	}
	w.Filter("нет такого")
	if w.Lessons == nil || len(w.Lessons) != 0 {
		t.Errorf("lessons = %#v, want an empty list", w.Lessons)
	}
}

func TestParseURL(t *testing.T) {
	cases := []struct {
		link string
		want Target
	}{
		{"https://ruz.spbstu.ru/places/33/967", Target{Kind: KindRoom, BuildingID: 33, ID: 967}},
		{"ruz.spbstu.ru/places/33/967?date=2026-11-9", Target{Kind: KindRoom, BuildingID: 33, ID: 967, Date: "2026-11-09"}},
		{"https://ruz.spbstu.ru/faculty/125/groups/45851", Target{Kind: KindGroup, ID: 45851}},
		{"https://ruz.spbstu.ru/teachers/10734/", Target{Kind: KindTeacher, ID: 10734}},
	}
	for _, c := range cases {
		got, err := ParseURL(c.link)
		if err != nil || got != c.want {
			t.Errorf("ParseURL(%q) = %+v, %v; want %+v", c.link, got, err, c.want)
		}
	}
	for _, bad := range []string{"https://dl.spbstu.ru/places/33/967", "https://ruz.spbstu.ru/", "https://ruz.spbstu.ru/places/33"} {
		if _, err := ParseURL(bad); err == nil {
			t.Errorf("ParseURL(%q) must fail", bad)
		}
	}
}
