package demo

import (
	"context"
	"strings"
	"time"

	"github.com/kad-xyz/spbstu-mcp/internal/ews"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
)

func (DL) Quizzes(_ context.Context, courseID int) ([]moodle.Quiz, error) {
	if courseID != courseFA {
		return nil, nil
	}
	best := "7 / 10"
	return []moodle.Quiz{
		{ID: 801, CMID: 9001, Name: "Тест: метрические и нормированные пространства", Closes: stamp(at(5, 18, 0)), TimeLimitMin: 30, Attempts: "2"},
		{ID: 802, CMID: 9005, Name: "Тренировочный тест: определения", TimeLimitMin: 20, Attempts: "без ограничений", Used: 3, BestGrade: &best},
	}, nil
}

func (DL) Teachers(_ context.Context, courseID int) ([]moodle.Person, error) {
	names := map[int]string{courseFA: "Орлов Дмитрий Константинович", courseDB: "Петрова Анна Сергеевна",
		courseMath: "Смирнова Елена Викторовна", coursePractice: "Кузнецов Павел Ильич"}
	return []moodle.Person{{ID: 3000 + courseID, Name: names[courseID], Role: "преподаватель", LastAccess: stamp(at(-1, 21, 5))}}, nil
}

func (DL) Notifications(context.Context, int) ([]moodle.Notification, int, error) {
	return []moodle.Notification{
		{Time: stamp(at(0, 8, 0)), Subject: "Срок сдачи «Лабораторная работа 3. Нормализация» через 2 дня"},
		{Time: stamp(at(-2, 14, 30)), Subject: "Петрова А. С. оценила «Лабораторная работа 2. ER-моделирование»: 4,00", Read: true},
	}, 1, nil
}

func (DL) Conversations(context.Context, int) ([]moodle.Conversation, error) {
	return []moodle.Conversation{{ID: 41, With: []string{"Петрова Анна Сергеевна"}, Members: []int{3102},
		Last: "Да, ER-диаграмму можно сделать в draw.io.", LastAt: stamp(at(-1, 19, 40))}}, nil
}

func (DL) ConversationMessages(context.Context, int) ([]moodle.Message, error) {
	return []moodle.Message{
		{From: "я", Time: stamp(at(-1, 18, 2)), Text: "Здравствуйте! Можно сделать ER-диаграмму в draw.io?"},
		{From: "Петрова Анна Сергеевна", Time: stamp(at(-1, 19, 40)), Text: "Да, ER-диаграмму можно сделать в draw.io."},
	}, nil
}

func (DL) SendMessage(context.Context, int, string) error { return errReadOnly }

func (d DL) Search(ctx context.Context, query string, _ bool) ([]moodle.Hit, error) {
	var out []moodle.Hit
	q := strings.ToLower(query)
	for _, co := range []int{courseDB} {
		secs, _ := d.Contents(ctx, co)
		for _, s := range secs {
			for _, m := range s.Modules {
				if strings.Contains(strings.ToLower(m.Name), q) {
					out = append(out, moodle.Hit{Course: "Базы данных (заочная форма обучения)", CourseID: co, Section: s.Name, Name: m.Name, Type: m.Type, CMID: m.CMID, InstanceID: m.InstanceID, Match: "name"})
				}
			}
		}
	}
	return out, nil
}

func (Mail) Folders(context.Context) ([]ews.Folder, error) {
	return []ews.Folder{{ID: "inbox", Name: "Входящие", Total: 128, Unread: 3}, {ID: "sentitems", Name: "Отправленные", Total: 41},
		{ID: "demo-study", Name: "Учёба", Total: 56}}, nil
}

func (Mail) Move(context.Context, []string, string) error   { return errReadOnly }
func (Mail) MarkRead(context.Context, []string, bool) error { return errReadOnly }

func (Mail) Calendar(context.Context, time.Time, time.Time) ([]ews.Event, error) {
	return []ews.Event{{Subject: "Консультация по базам данных (вебинар)", Start: stamp(at(3, 18, 0)), End: stamp(at(3, 19, 30)),
		Location: "Яндекс Телемост", Organizer: "Петрова Анна Сергеевна <petrova_as@spbstu.ru>"}}, nil
}
