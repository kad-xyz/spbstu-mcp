package answers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const vspPage = `<html><body><article><div class="post-text">
<table class="tablepress"><thead><tr class="row-1"><th class="column-1">ВОПРОС</th><th class="column-2">ОТВЕТ</th></tr></thead>
<tbody>
<tr class="row-2"><td class="column-1">При помощи каких устройств осуществляется организованная естественная вентиляция?</td><td class="column-2">окна, фрамуги, дефлекторы</td></tr>
<tr class="row-3"><td class="column-1">Лазер 2-го класса – это лазер, у которого</td><td class="column-2">Коллимированное излучение опасно для глаз<br />
Зеркально отраженное излучение опасно для глаз</td></tr>
<tr class="row-4"><td class="column-1">Напряжённость электрического поля измеряется в</td><td class="column-2">В перечисленных ответах нет правильного</td></tr>
</tbody></table></div></article></body></html>`

const phisPage = `<html><body><div class="item2">
<h1>Ответы на тесты по теме БЖД</h1>
<p></p><hr>
<strong>Вытяжное устройство для удаления загрязненного воздуха из помещений, устанавливаемое на крыше здания – это?</strong>
<p>4. дефлектор</p>
<hr>
<strong>Верно ли утверждение, что возможно применять «защиту временем»?</strong>
<p>Верно</p>
<hr>
<div class="pagination"><a> Страница 1 из 129. </a></div>
</div></body></html>`

func TestParseVSP(t *testing.T) {
	e := parseVSP([]byte(vspPage), "u")
	if len(e) != 3 {
		t.Fatalf("want 3 entries, got %d: %v", len(e), e)
	}
	if e[1].Answer != "Коллимированное излучение опасно для глаз; Зеркально отраженное излучение опасно для глаз" {
		t.Errorf("cell lines must be joined: %q", e[1].Answer)
	}
}

func TestParsePhis(t *testing.T) {
	e, pages := parsePhis([]byte(phisPage), "u")
	if pages != 129 || len(e) != 2 || e[0].Answer != "4. дефлектор" || e[1].Answer != "Верно" {
		t.Errorf("got %d pages, %v", pages, e)
	}
}

func TestBest(t *testing.T) {
	pool := parseVSP([]byte(vspPage), "u")
	r := best("Напряженность электрического поля измеряется в:", pool)
	if !r.Found || r.Matches[0].Answer != "В перечисленных ответах нет правильного" {
		t.Errorf("ё and punctuation must not matter: %+v", r)
	}
	r = best("Какие устройства используются для организованной естественной вентиляции помещений?", pool)
	if len(r.Matches) == 0 || r.Matches[0].Answer != "окна, фрамуги, дефлекторы" {
		t.Errorf("reworded question must still be offered: %+v", r)
	}
	r = best("Что такое валютный паритет?", pool)
	if r.Found || len(r.Matches) != 0 {
		t.Errorf("an unrelated question must not match: %+v", r)
	}
	r = best("Лазер 2-го класса – это лазер, у которого (выберите несколько ответов)", pool)
	if !r.Found {
		t.Errorf("a question with an instruction added must be found: %+v", r)
	}
}

func TestFragments(t *testing.T) {
	f := fragments("Вытяжное устройство для удаления загрязненного воздуха – это?")
	if f[0] != "Вытяжное устройство для" {
		t.Errorf("first fragment is the start of the question: %v", f)
	}
	if !contains(f, "загрязненного воздуха") {
		t.Errorf("the longest word goes with the next one: %v", f)
	}
}

func TestSubjectURL(t *testing.T) {
	for in, want := range map[string]string{
		"bgd":                        "https://polyvsp.ru/bgd/",
		"https://polyvsp.ru/bgd":     "https://polyvsp.ru/bgd/",
		"polyphis.ru/bzd":            "https://polyphis.ru/bzd",
		"https://polyphis.ru/bzd?x=": "https://polyphis.ru/bzd",
	} {
		if got, err := subjectURL(in); err != nil || got != want {
			t.Errorf("%s: got %s %v", in, got, err)
		}
	}
	if _, err := subjectURL("https://example.com/x"); err == nil {
		t.Error("other sites are not answer bases")
	}
}

func TestFetchCacheAndStale(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits > 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte("page"))
	}))
	defer srv.Close()
	c := &Client{CacheDir: t.TempDir(), TTL: time.Hour}
	ctx := context.Background()
	for range 2 {
		if b, err := c.fetch(ctx, srv.URL); err != nil || string(b) != "page" {
			t.Fatalf("got %q %v", b, err)
		}
	}
	if hits != 1 {
		t.Errorf("a fresh page must come from the cache, server hit %d times", hits)
	}
	c.TTL = 0
	if b, err := c.fetch(ctx, srv.URL); err != nil || string(b) != "page" {
		t.Errorf("a stale copy must be used when the site fails: %q %v", b, err)
	}
	if files, _ := filepath.Glob(filepath.Join(c.CacheDir, "*.html")); len(files) != 1 {
		t.Errorf("one cached file expected, got %v", files)
	}
	_ = os.RemoveAll(c.CacheDir)
	if _, err := c.fetch(ctx, srv.URL); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("without a cache the error must come through: %v", err)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
