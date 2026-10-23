// Package demo serves made-up but realistic data in place of the university services, for
// trying the server out without an account and for recording demos. Nothing here is real.
package demo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kad-xyz/spbstu-mcp/internal/ews"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
	"github.com/kad-xyz/spbstu-mcp/internal/openedu"
)

// at is a time relative to now, days ahead (negative: ago) at hh:mm.
func at(days, hh, mm int) time.Time {
	d := time.Now().AddDate(0, 0, days)
	return time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, time.Local)
}

func stamp(t time.Time) string { return t.Format("2006-01-02 15:04") }

const (
	courseDB       = 102
	courseFA       = 101
	courseMath     = 103
	coursePractice = 104
	assignLab3     = 5003
	fileBase       = "https://dl.spbstu.ru/webservice/pluginfile.php/demo/"
)

var errReadOnly = errors.New("demo mode: nothing was sent")

// DL is the dl.spbstu.ru side of the demo.
type DL struct{}

func (DL) Courses(context.Context, bool) ([]moodle.Course, error) {
	return []moodle.Course{
		{ID: courseFA, Name: "Введение в функциональный анализ", ShortName: "ВФА"},
		{ID: courseDB, Name: "Базы данных (заочная форма обучения)", ShortName: "БД_з"},
		{ID: courseMath, Name: "Математическое обеспечение информационных систем", ShortName: "МОИС"},
		{ID: coursePractice, Name: "Ознакомительная практика", ShortName: "Практика"},
	}, nil
}

func (DL) Deadlines(context.Context, int, int) ([]moodle.Deadline, error) {
	return []moodle.Deadline{
		{Due: stamp(at(-3, 23, 59)), Course: "Ознакомительная практика", CourseID: coursePractice,
			Activity: "Отчёт по практике и три записи в дневнике", Kind: "Задание - срок сдачи",
			URL: "https://dl.spbstu.ru/mod/assign/view.php?id=9104", Overdue: true},
		{Due: stamp(at(2, 23, 59)), Course: "Базы данных (заочная форма обучения)", CourseID: courseDB,
			Activity: "Лабораторная работа 3. Нормализация", Kind: "Задание - срок сдачи",
			URL: "https://dl.spbstu.ru/mod/assign/view.php?id=9003"},
		{Due: stamp(at(5, 18, 0)), Course: "Введение в функциональный анализ", CourseID: courseFA,
			Activity: "Тест: метрические и нормированные пространства", Kind: "Тест закрывается",
			URL: "https://dl.spbstu.ru/mod/quiz/view.php?id=9001"},
		{Due: stamp(at(12, 23, 59)), Course: "Математическое обеспечение информационных систем", CourseID: courseMath,
			Activity: "Практическое задание: оптимизация методом ветвей и границ", Kind: "Задание - срок сдачи",
			URL: "https://dl.spbstu.ru/mod/assign/view.php?id=9013"},
	}, nil
}

func (DL) Assignments(_ context.Context, courseID int) ([]moodle.Assignment, error) {
	if courseID != courseDB {
		return nil, nil
	}
	return []moodle.Assignment{{
		ID: assignLab3, CMID: 9003, Name: "Лабораторная работа 3. Нормализация",
		Due: stamp(at(2, 23, 59)), Cutoff: stamp(at(9, 23, 59)),
		Description: "Приведите схему базы данных «Библиотека» из лабораторной работы 2 к третьей нормальной форме.\n" +
			"1. Выпишите функциональные зависимости для каждой таблицы.\n" +
			"2. Покажите шаги приведения к 1НФ, 2НФ и 3НФ с обоснованием.\n" +
			"3. Нарисуйте итоговую ER-диаграмму (draw.io или аналог).\n" +
			"4. Приложите SQL-скрипт создания таблиц с первичными и внешними ключами.\n" +
			"Отчёт — один PDF до 10 страниц, имя файла: Группа_Фамилия_ЛР3.pdf. " +
			"Работы после срока принимаются в течение недели с понижением на 1 балл.",
		Files: []moodle.File{
			{Name: lab3Guide, URL: fileBase + lab3Guide, Size: int64(len(files[lab3Guide]))},
			{Name: lab2Schema, URL: fileBase + lab2Schema, Size: int64(len(files[lab2Schema]))},
		},
	}}, nil
}

func (DL) Contents(_ context.Context, courseID int) ([]moodle.Section, error) {
	if courseID != courseDB {
		return []moodle.Section{{Name: "Общее", Modules: []moodle.Module{{CMID: 9900, InstanceID: 900, Name: "Объявления", Type: "forum"}}}}, nil
	}
	return []moodle.Section{
		{Name: "Общее", Modules: []moodle.Module{
			{CMID: 9000, InstanceID: 900, Name: "Объявления", Type: "forum"},
			{CMID: 9010, InstanceID: 910, Name: "Порядок изучения курса и требования к экзамену", Type: "resource",
				Files: []moodle.File{{Name: examRules, URL: fileBase + examRules, Size: int64(len(files[examRules]))}}},
		}},
		{Name: "Лабораторные работы", Modules: []moodle.Module{
			{CMID: 9002, InstanceID: 5002, Name: "Лабораторная работа 2. ER-моделирование", Type: "assign"},
			{CMID: 9003, InstanceID: assignLab3, Name: "Лабораторная работа 3. Нормализация", Type: "assign"},
		}},
	}, nil
}

func (DL) SubmissionStatus(context.Context, int) (map[string]any, error) {
	return map[string]any{"status": "new", "grading_status": "notgraded", "can_edit": true}, nil
}

func (DL) Grades(context.Context, int) ([]moodle.GradeItem, error) {
	return []moodle.GradeItem{
		{Item: "Лабораторная работа 1. SQL-запросы", Grade: "5,00", Range: "0–5"},
		{Item: "Лабораторная работа 2. ER-моделирование", Grade: "4,00", Range: "0–5", Feedback: "Нет внешнего ключа в таблице «Выдачи»."},
		{Item: "Лабораторная работа 3. Нормализация", Grade: "-", Range: "0–5"},
	}, nil
}

func (DL) Discussions(context.Context, int) ([]moodle.Discussion, error) {
	return []moodle.Discussion{{ID: 7001, Subject: "Консультация перед экзаменом", Author: "Петрова Анна Сергеевна",
		Created: stamp(at(-2, 10, 15)), Message: "Консультация пройдёт онлайн за два дня до экзамена, ссылку пришлю сюда."}}, nil
}

func (DL) Posts(context.Context, int) ([]moodle.Post, error) {
	return []moodle.Post{{ID: 70011, Author: "Петрова Анна Сергеевна", Created: stamp(at(-2, 10, 15)),
		Subject: "Консультация перед экзаменом", Message: "Консультация пройдёт онлайн за два дня до экзамена, ссылку пришлю сюда."}}, nil
}

func (DL) Page(context.Context, int, int) (string, error) {
	return "Экзамен проходит тестом на dl: 20 вопросов, 60 минут, одна попытка.", nil
}

func (DL) Download(_ context.Context, fileURL, dir string) (string, error) {
	name := filepath.Base(fileURL)
	content, ok := files[name]
	if !ok {
		return "", fmt.Errorf("demo: no file %s", name)
	}
	return save(dir, name, content)
}

func save(dir, name, content string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, []byte(content), 0o644)
}

func (DL) SaveSubmission(context.Context, int, []string, string, bool) error { return errReadOnly }
func (DL) AddDiscussion(context.Context, int, string, string, []string) (int, error) {
	return 0, errReadOnly
}
func (DL) Reply(context.Context, int, string, string, []string) error { return errReadOnly }

// Mail is the mailbox side of the demo.
type Mail struct{}

var messages = []ews.Message{
	{ID: "demo-1", Subject: "Расписание зимней сессии", From: "Соколова Мария Андреевна <sokolova_ma@spbstu.ru>",
		To: []string{"Иванов Иван Иванович <ivanov.ii@edu.spbstu.ru>"},
		Body: "Здравствуйте! Расписание сессии опубликовано в личном кабинете. Обратите внимание: " +
			"экзамен по базам данных перенесён на первую неделю сессии. Куратор заочного отделения М. А. Соколова."},
	{ID: "demo-2", Subject: "ЛР3: формат отчёта", From: "Петрова Анна Сергеевна <petrova_as@spbstu.ru>",
		To: []string{"Иванов Иван Иванович <ivanov.ii@edu.spbstu.ru>"}, HasAttach: true,
		Body: "Коллеги, в отчёте по ЛР3 обязательно покажите функциональные зависимости до и после декомпозиции. " +
			"Пример оформления во вложении.",
		Attachments: []ews.Attachment{{ID: "demo-att-1", Name: reportSample, Size: len(files[reportSample])}}},
	{ID: "demo-3", Subject: "Срок сдачи «Лабораторная работа 3. Нормализация»", From: "dl.spbstu.ru <noreply@spbstu.ru>",
		To:   []string{"Иванов Иван Иванович <ivanov.ii@edu.spbstu.ru>"},
		Body: "Срок сдачи задания «Лабораторная работа 3. Нормализация» истекает через 2 дня."},
}

func init() {
	for i, d := range []int{-1, -1, 0} {
		messages[i].Received = stamp(at(d, 9+3*i, 20))
	}
}

func (Mail) List(_ context.Context, folder string, limit int, _ bool, _ string) ([]ews.Message, error) {
	if folder != "" && folder != "inbox" {
		return []ews.Message{}, nil
	}
	out := make([]ews.Message, 0, len(messages))
	for i := len(messages) - 1; i >= 0 && len(out) < limit; i-- {
		m := messages[i]
		m.Preview, m.Body, m.Attachments = firstWords(m.Body), "", nil
		out = append(out, m)
	}
	return out, nil
}

func (Mail) Read(_ context.Context, id string, markRead bool) (ews.Message, error) {
	for i, m := range messages {
		if m.ID == id {
			if markRead {
				messages[i].Read = true
				m.Read = true
			}
			return m, nil
		}
	}
	return ews.Message{}, fmt.Errorf("message %s not found", id)
}

func (Mail) Attachment(context.Context, string) (string, []byte, error) {
	return reportSample, []byte(files[reportSample]), nil
}

func (Mail) Submit(context.Context, ews.Draft) error { return errReadOnly }
func (Mail) Delete(context.Context, string) error    { return errReadOnly }

func firstWords(s string) string {
	r := []rune(s)
	if len(r) > 90 {
		return string(r[:90]) + "…"
	}
	return s
}

// OpenEdu is the openedu.ru side of the demo.
type OpenEdu struct{}

const courseDATAM = "course-v1:spbstu+DATAM+demo"

func (OpenEdu) Courses(context.Context, bool) ([]openedu.Course, error) {
	return []openedu.Course{{ID: courseDATAM, Name: "Управление данными", Start: stamp(at(-45, 0, 0)), End: stamp(at(230, 0, 0)), Active: true}}, nil
}

func (OpenEdu) Progress(context.Context, string) (*openedu.Progress, error) {
	return &openedu.Progress{Complete: 48, Incomplete: 79, Grade: "31%", Passing: false,
		Policy:   []string{"Homework: 16 шт., вес 20%, проходной 75%, можно не сдавать 0", "Final Exam: 4 шт., вес 60%, проходной 50%, можно не сдавать 3"},
		Sections: []openedu.Section{{Name: "Неделя 4. Нормализация", Subsections: []openedu.Subsection{{Name: "Неделя 4. Тест", Type: "Homework", Earned: "3/5", Graded: true}}}},
	}, nil
}

func (OpenEdu) Dates(context.Context, string) ([]openedu.Date, error) {
	return []openedu.Date{{Date: stamp(at(6, 23, 59)), Title: "Неделя 5. Тест", Type: "Homework"}}, nil
}

func (OpenEdu) Units(context.Context, string, string) ([]openedu.Unit, error) {
	return []openedu.Unit{{Name: "Неделя 5. Лабораторная SQL Server", ID: "block-v1:spbstu+DATAM+demo+type@vertical+block@lab5",
		Text:  "ЛАБОРАТОРНАЯ РАБОТА ПЯТОЙ НЕДЕЛИ\nОсновы управления транзакциями в SQL Server.\nОписание работы.",
		Links: []openedu.UnitLink{{Text: "Описание работы", URL: "https://cdn2.openedu.ru/assets/demo/lab5.pdf"}}}}, nil
}

func (OpenEdu) Outline(context.Context, string) ([]openedu.Chapter, error) {
	return []openedu.Chapter{{Name: "Неделя 5. Транзакции", Items: []openedu.OutlineItem{{Name: "Неделя 5. Тест", Due: stamp(at(6, 23, 59))}}}}, nil
}

func demoProblem() openedu.Problem {
	return openedu.Problem{ID: "block-v1:spbstu+DATAM+demo+type@problem+block@q51", Unit: "Неделя 5. Тест", Title: "Вопрос 5.1",
		Score: "0/1", Attempts: "0/2", Graded: true,
		Inputs: []openedu.Input{{Name: "input_q51_2_1", Type: "radio", Status: "unanswered",
			Prompt: "Какое свойство транзакции гарантирует, что после фиксации её результат не потеряется?",
			Choices: []openedu.Choice{{Value: "choice_0", Label: "Атомарность"}, {Value: "choice_1", Label: "Изолированность"},
				{Value: "choice_2", Label: "Долговечность"}}}}}
}

func (OpenEdu) Problems(context.Context, string, string) ([]openedu.Problem, error) {
	return []openedu.Problem{demoProblem()}, nil
}

func (OpenEdu) Answer(_ context.Context, _, _ string, answers map[string][]string, submit bool) (*openedu.Problem, error) {
	p := demoProblem()
	p.Inputs[0].Answer = answers["input_q51_2_1"]
	p.Inputs[0].Status = "unsubmitted"
	if submit {
		p.Attempts, p.Inputs[0].Status = "1/2", "incorrect"
		if len(p.Inputs[0].Answer) == 1 && p.Inputs[0].Answer[0] == "choice_2" {
			p.Score, p.Inputs[0].Status = "1/1", "correct"
		}
	}
	return &p, nil
}

func demoExam(status string) *openedu.Exam {
	return &openedu.Exam{ID: 1, Name: "Промежуточный экзамен", Type: "timed", TimeLimit: "45 мин.", Due: stamp(at(30, 23, 59)), Status: status}
}

func (OpenEdu) Exam(_ context.Context, _, _ string, start bool) (*openedu.Exam, error) {
	if !start {
		e := demoExam("not started")
		e.Notice = "starting runs the clock for 45 мин. and cannot be undone"
		return e, nil
	}
	e := demoExam("started")
	e.AttemptID, e.MinutesLeft = 1, 45
	p := demoProblem()
	p.Unit = "Промежуточный экзамен"
	e.Problems = []openedu.Problem{p}
	return e, nil
}

func (OpenEdu) Enroll(_ context.Context, courseID string) (*openedu.Course, error) {
	return &openedu.Course{ID: courseID, Name: "Введение в Python", Start: stamp(at(-45, 0, 0)), End: stamp(at(60, 0, 0)), Active: true}, nil
}

func (OpenEdu) FinishExam(context.Context, string, string) (*openedu.Exam, error) {
	e := demoExam("submitted")
	e.AttemptID = 1
	return e, nil
}
