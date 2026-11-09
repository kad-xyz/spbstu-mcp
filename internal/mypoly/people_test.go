package mypoly

import (
	"encoding/json"
	"testing"
)

func TestParsePeople(t *testing.T) {
	body := `{"results":[
		{"id":297920,"full_name":"Иванов Иван Иванович","info":"ИКНК, гр.з5130902/30203","type_person":"s","text":"x"},
		{"id":1,"full_name":"Петров Пётр","info":"Высшая школа, доцент","type_person":"e"}]}`
	var raw rawPeople
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatal(err)
	}
	p := parsePeople(raw)
	if len(p) != 2 {
		t.Fatalf("got %d people", len(p))
	}
	if p[0].Kind != "student" || p[0].Unit != "ИКНК" || p[0].Group != "з5130902/30203" || p[0].Info != "" {
		t.Errorf("student: %+v", p[0])
	}
	if p[1].Kind != "employee" || p[1].Group != "" || p[1].Info != "Высшая школа, доцент" {
		t.Errorf("employee: %+v", p[1])
	}
}
