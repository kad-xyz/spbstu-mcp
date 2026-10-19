package mypoly

import (
	"encoding/json"
	"testing"
)

func TestFlattenWorkPlan(t *testing.T) {
	raw := `[{"block_name":"Б1.В.1","disciplines":[
		{"name":"Б1.В.1.01 Функциональный анализ","hours":144,"control_type":"экзамен","lec":2,"pract":2.0,"lab":null,"self":59,"children":[]},
		{"name":"Б1.В.1.ДВ.01 Дисциплины по выбору","hours":0,"control_type":null,"children":[
			{"name":"Б1.В.1.ДВ.01.01 Защита информации","hours":108,"control_type":"зачет","semesters":"8,9","children":[]}]}]}]`
	var blocks []rawBlock
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		t.Fatal(err)
	}
	var ds []Discipline
	for _, b := range blocks {
		flatten(b.BlockName, "", b.Disciplines, &ds)
	}
	if len(ds) != 2 {
		t.Fatalf("got %d disciplines", len(ds))
	}
	if ds[0].Practice != 2 || ds[0].Labs != 0 || ds[0].Control != "экзамен" {
		t.Errorf("hours or control lost: %+v", ds[0])
	}
	if ds[1].Group != "Б1.В.1.ДВ.01 Дисциплины по выбору" || !inSemester(ds[1].Semesters, 9) || inSemester(ds[1].Semesters, 7) {
		t.Errorf("elective group or semesters wrong: %+v", ds[1])
	}
}

func TestRecordBookOptions(t *testing.T) {
	page := `<select class="form-select" id="recordBook">
		<option value="123">з5130000/00001</option>
		<option value="456" selected>5130000/00002</option>
	</select><select id="other"><option value="9">x</option></select>`
	m := recordBookRe.FindAllStringSubmatch(page[:len(page)-50], -1)
	if len(m) != 2 || m[0][1] != "123" || m[1][2] != "5130000/00002" {
		t.Fatalf("options: %v", m)
	}
}
