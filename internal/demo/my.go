package demo

import (
	"context"

	"github.com/kad-xyz/spbstu-mcp/internal/mypoly"
)

// My is the my.spbstu.ru side of the demo.
type My struct{}

var plan = []mypoly.Discipline{
	{Name: "Б1.В.1.01 Введение в функциональный анализ", Semesters: "5,6,7", Hours: 144, Control: "экзамен"},
	{Name: "Б1.В.1.04 Базы данных", Semesters: "6,7", Hours: 180, Control: "зачет, экзамен"},
	{Name: "Б1.В.1.07 Математическое обеспечение информационных систем", Semesters: "7", Hours: 144, Control: "экзамен"},
	{Name: "Б2.О.01(У) Ознакомительная практика", Semesters: "7", Hours: 108, Control: "дифф. зачет"},
}

func (My) RecordBooks(context.Context) ([]mypoly.RecordBook, error) {
	return []mypoly.RecordBook{{ID: "1000001", Group: "з5130000/00001"}}, nil
}

func (My) Grades(_ context.Context, _ string, semester int) (*mypoly.Grades, error) {
	all := []mypoly.Grade{
		{Semester: 5, Discipline: "Дополнительные главы математики", Control: "экзамен", Grade: "хорошо", Date: "18.01.2026", Lecturer: "Смирнова Е.В."},
		{Semester: 6, Discipline: "Базы данных", Control: "зачёт", Grade: "зачтено", Date: "02.06.2026", Lecturer: "Петрова А.С."},
		{Semester: 6, Discipline: "Теория информационных систем", Control: "экзамен", Grade: "отлично", Date: "10.06.2026", Lecturer: "Орлов Д.К."},
	}
	g := &mypoly.Grades{Averages: map[string]float64{"5": 4.2, "6": 4.5}}
	for _, x := range all {
		if semester <= 0 || x.Semester == semester {
			g.Grades = append(g.Grades, x)
		}
	}
	return g, nil
}

func (My) StudyPlan(_ context.Context, _ string, semester int) ([]mypoly.Discipline, error) {
	if semester <= 0 {
		return plan, nil
	}
	var out []mypoly.Discipline
	for _, d := range plan {
		for _, s := range d.Semesters {
			if string(s) == string(rune('0'+semester)) {
				out = append(out, d)
				break
			}
		}
	}
	return out, nil
}

func (My) WorkPlan(context.Context, string, int) (map[string][]mypoly.Discipline, error) {
	return map[string][]mypoly.Discipline{"7": {
		{Name: "Б1.В.1.01 Введение в функциональный анализ", Hours: 144, Control: "экзамен", Lectures: 2, Practice: 2, SelfStudy: 59},
		{Name: "Б1.В.1.04 Базы данных", Hours: 180, Control: "экзамен", Lectures: 4, Labs: 4, SelfStudy: 80},
	}}, nil
}
