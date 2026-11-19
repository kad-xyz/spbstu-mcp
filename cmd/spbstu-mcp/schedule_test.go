package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kad-xyz/spbstu-mcp/internal/ruz"
)

func TestPick(t *testing.T) {
	groups := []ruz.Match{{ID: 1, Name: "5130902/30201"}, {ID: 2, Name: "з5130902/30201"}}
	if m, many := pick(groups, "З5130902/30201"); many != nil || m.ID != 2 {
		t.Errorf("exact match ignoring case: got %v %v", m, many)
	}
	if _, many := pick(groups, "5130902"); many == nil || len(many.Matches) != 2 {
		t.Error("several matches must be returned to choose from")
	}
	if m, many := pick(groups[:1], "5130902"); many != nil || m.ID != 1 {
		t.Error("a single match must be taken")
	}
	if _, many := pick(nil, "x"); many == nil || many.Note != "Nothing found." {
		t.Error("no match must say so")
	}
}

func TestResolveGroup(t *testing.T) {
	ctx := context.Background()
	own := func(context.Context) (string, error) { return "з5130902/30201", nil }
	none := func(context.Context) (string, error) { return "", errors.New("my.spbstu.ru: no record book found") }
	called := false
	spy := func(context.Context) (string, error) { called = true; return "x", nil }

	if g, _ := resolveGroup(ctx, "5130902/30201", spy, "flag"); g != "5130902/30201" || called {
		t.Errorf("an asked group must be used as is, got %q (record book read: %v)", g, called)
	}
	if g, _ := resolveGroup(ctx, "", own, "flag"); g != "з5130902/30201" {
		t.Errorf("the record book group must come before the flag, got %q", g)
	}
	if g, _ := resolveGroup(ctx, "", none, "flag"); g != "flag" {
		t.Errorf("the flag must be the fallback, got %q", g)
	}
	if _, err := resolveGroup(ctx, "", none, ""); err == nil || !strings.Contains(err.Error(), "no record book") {
		t.Errorf("err = %v, want the reason the group is unknown", err)
	}
}

func TestTrim(t *testing.T) {
	w := &ruz.Week{Of: "DL, ауд. Дистанционно", Lessons: make([]ruz.Lesson, maxLessons+5)}
	trim(w, "")
	if len(w.Lessons) != maxLessons || !strings.Contains(w.Of, "give a query") {
		t.Errorf("got %d lessons, of %q", len(w.Lessons), w.Of)
	}
}

func TestScheduleChecksWeeks(t *testing.T) {
	if _, _, err := schedule(context.Background(), scheduleIn{URL: "ruz.spbstu.ru/places/33/967", Weeks: 9}, nil, ""); err == nil {
		t.Error("weeks above 8 must be refused")
	}
}
