package openedu

import (
	"reflect"
	"testing"
)

func TestGradedOnly(t *testing.T) {
	p := &Progress{Grade: "20%", Sections: []Section{
		{Name: "Неделя 1", Subsections: []Subsection{{Name: "Видео"}, {Name: "Тест", Graded: true, Earned: "5/5"}}},
		{Name: "Общая информация", Subsections: []Subsection{{Name: "Сертификаты"}}},
		{Name: "Модуль мобильности", Subsections: []Subsection{{Name: "Обязательно к прочтению", Earned: "0/3"}}},
	}}
	got := p.GradedOnly()
	want := []Section{
		{Name: "Неделя 1", Subsections: []Subsection{{Name: "Тест", Graded: true, Earned: "5/5"}}},
		{Name: "Модуль мобильности", Subsections: []Subsection{{Name: "Обязательно к прочтению", Earned: "0/3"}}},
	}
	if !reflect.DeepEqual(got.Sections, want) || got.Grade != "20%" {
		t.Errorf("got %+v", got)
	}
	if len(p.Sections) != 3 {
		t.Error("GradedOnly changed the original")
	}
}
