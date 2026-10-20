package answers

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// TestLive lists the courses and looks a question up on both sites, using the real cache.
// Run with SPBSTU_LIVE=1.
func TestLive(t *testing.T) {
	if os.Getenv("SPBSTU_LIVE") == "" {
		t.Skip("set SPBSTU_LIVE=1 to run against polyvsp.ru and polyphis.ru")
	}
	c := Default()
	ctx := context.Background()
	subj, err := c.Subjects(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	sites := map[string]int{}
	for _, s := range subj {
		sites[s.Site]++
	}
	t.Logf("courses: %v", sites)
	res, notes, err := c.Search(ctx, []string{
		"При помощи каких устройств осуществляется организованная естественная вентиляция?",
		"Вытяжное устройство для удаления загрязненного воздуха из помещений, устанавливаемое на крыше здания – это?",
	}, []string{"bgd", "polyphis.ru/bzd"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(map[string]any{"results": res, "notes": notes}, "", " ")
	t.Log(string(b))
	for _, r := range res {
		if !r.Found {
			t.Errorf("not found: %s", r.Question)
		}
	}
}
