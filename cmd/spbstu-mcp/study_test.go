package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFailing(t *testing.T) {
	for g, want := range map[string]bool{"неудовлетворительно": true, "не зачтено": true, "неявка": true,
		"зачтено": false, "удовлетворительно": false, "отлично": false} {
		if failing(g) != want {
			t.Errorf("failing(%q) = %v", g, !want)
		}
	}
}

func TestNormName(t *testing.T) {
	if got := normName("Б1.В.1.06 Аппаратное обеспечение"); got != "аппаратное обеспечение" {
		t.Errorf("normName = %q", got)
	}
	if got := normName("Б2.О.01(У) Ознакомительная практика"); got != "ознакомительная практика" {
		t.Errorf("normName = %q", got)
	}
}

func TestLaterDate(t *testing.T) {
	if !laterDate("02.06.2026", "31.01.2026") || laterDate("31.01.2026", "02.06.2026") || !laterDate("01.01.2026", "") {
		t.Error("laterDate")
	}
}

func TestWriteICS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.ics")
	soon := time.Now().Add(48 * time.Hour).Format("2006-01-02 15:04")
	past := time.Now().Add(-48 * time.Hour).Format("2006-01-02 15:04")
	n, err := writeICS(path, []icsEvent{
		{Title: "ЛР3, Нормализация; БД", When: soon, URL: "https://dl.spbstu.ru/x"},
		{Title: "old", When: past},
		{Title: "bad", When: "never"},
	}, time.Now().AddDate(0, 0, 30))
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	for _, want := range []string{"BEGIN:VCALENDAR", `SUMMARY:ЛР3\, Нормализация\; БД`, "TRIGGER:-P1D", "END:VCALENDAR"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
}
