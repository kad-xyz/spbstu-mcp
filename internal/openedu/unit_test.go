package openedu

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

func TestPageText(t *testing.T) {
	page := `<html><head><title>x</title><script>var a = "skip";</script></head><body>
<a class="nav-skip sr-only" href="#main">Перейти к основному содержимому</a><div class="xblock"><script type="json/xblock-args">{"display_name": "Видео"}</script>
<p>ЛАБОРАТОРНАЯ&nbsp;РАБОТА  № 1</p><ul><li>Шаг <b>один</b></li><li>Шаг два</li></ul>
<p><a href="/assets/courseware/v1/abc/lab.pdf">Описание работы.</a> <a href="#top">наверх</a>
<a href="https://cdn2.openedu.ru/x.zip">Скрипт</a> <a href="/assets/courseware/v1/abc/lab.pdf">ещё раз</a></p>
<button>Закладка</button></div></body></html>`
	text, links := pageText([]byte(page))
	want := "ЛАБОРАТОРНАЯ РАБОТА № 1\n- Шаг один\n- Шаг два\nОписание работы. наверх\nСкрипт ещё раз"
	if text != want {
		t.Errorf("text:\n%q\nwant\n%q", text, want)
	}
	if len(links) != 2 || links[0].URL != LMSURL+"/assets/courseware/v1/abc/lab.pdf" ||
		links[0].Text != "Описание работы." || links[1].URL != "https://cdn2.openedu.ru/x.zip" {
		t.Errorf("links: %+v", links)
	}
}

func TestFindBlock(t *testing.T) {
	blocks := map[string]courseBlock{
		"block-v1:c+type@sequential+block@aa11": {ID: "block-v1:c+type@sequential+block@aa11", Type: "sequential", Name: "Неделя 9. Занятия"},
		"block-v1:c+type@vertical+block@bb22":   {ID: "block-v1:c+type@vertical+block@bb22", Type: "vertical", Name: "Неделя 9. Лабораторные SQL Server"},
		"block-v1:c+type@vertical+block@cc33":   {ID: "block-v1:c+type@vertical+block@cc33", Type: "vertical", Name: "Неделя 9. Лабораторные Oracle"},
	}
	for ref, want := range map[string]string{
		"block-v1:c+type@vertical+block@bb22":                                                "bb22",
		"https://courses.openedu.ru/courses/c/jump_to/block-v1:c+type@sequential+block@aa11": "aa11",
		"cc33":                    "cc33",
		"лабораторные sql server": "bb22",
	} {
		b, err := findBlock(blocks, ref)
		if err != nil || !strings.HasSuffix(b.ID, want) {
			t.Errorf("findBlock(%q) = %v, %v; want …%s", ref, b.ID, err, want)
		}
	}
	if _, err := findBlock(blocks, "Неделя 9"); err == nil || !strings.Contains(err.Error(), "matches 3") {
		t.Errorf("ambiguous name: %v", err)
	}
	if _, err := findBlock(blocks, "Неделя 42"); err == nil {
		t.Error("unknown name: want an error")
	}
}

// TestLiveUnits reads a lab page of the user's DATAM course. Run with SPBSTU_LIVE=1.
func TestLiveUnits(t *testing.T) {
	if os.Getenv("SPBSTU_LIVE") == "" {
		t.Skip("set SPBSTU_LIVE=1 to run against openedu.ru")
	}
	c := &Client{KeychainService: "spbstu", State: state.Default()}
	units, err := c.Units(context.Background(), "course-v1:spbstu+DATAM+fall_2026_spring_2027", "Неделя 14. Лабораторная SQL Server")
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || !strings.Contains(units[0].Text, "ЧЕТЫРНАДЦАТОЙ") || len(units[0].Links) == 0 {
		t.Fatalf("unexpected: %+v", units)
	}
	t.Logf("%s\n%s\n%+v", units[0].Name, units[0].Text, units[0].Links)
}
