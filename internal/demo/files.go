package demo

const (
	lab3Guide    = "ЛР3_Нормализация_методичка.md"
	lab2Schema   = "Библиотека_ЛР2.sql"
	examRules    = "Требования_к_экзамену.md"
	reportSample = "Пример_отчёта_ЛР3.md"
)

// files are the demo course files and attachments, served by dl_download and
// mail_download_attachment under these names.
var files = map[string]string{
	lab3Guide: `# Лабораторная работа 3. Нормализация

Цель: научиться находить функциональные зависимости и устранять аномалии вставки,
обновления и удаления, приводя схему к третьей нормальной форме.

## Исходные данные
Схема «Библиотека» из лабораторной работы 2 (файл Библиотека_ЛР2.sql).

## Порядок выполнения
1. Для каждой таблицы выпишите функциональные зависимости (ФЗ) и потенциальные ключи.
2. Найдите нарушения 1НФ (неатомарные значения), 2НФ (зависимость от части ключа)
   и 3НФ (транзитивные зависимости).
3. Выполните декомпозицию без потерь; для каждого шага укажите, какую ФЗ он устраняет.
4. Постройте ER-диаграмму итоговой схемы.
5. Напишите DDL-скрипт с PRIMARY KEY, FOREIGN KEY и ограничениями NOT NULL / UNIQUE.

## Критерии оценки
- ФЗ выписаны полно и верно — 2 балла;
- декомпозиция без потерь и с сохранением зависимостей — 2 балла;
- DDL и ER-диаграмма согласованы — 1 балл.
`,

	lab2Schema: `-- Схема «Библиотека» (ЛР 2). Сознательно не нормализована: это исходные данные для ЛР 3.

CREATE TABLE loans (
    loan_id        INTEGER PRIMARY KEY,
    reader_card    VARCHAR(10)  NOT NULL,
    reader_name    VARCHAR(100) NOT NULL,   -- зависит от reader_card
    reader_phones  VARCHAR(200),            -- несколько телефонов через запятую
    book_isbn      VARCHAR(17)  NOT NULL,
    book_title     VARCHAR(200) NOT NULL,   -- зависит от book_isbn
    book_authors   VARCHAR(300) NOT NULL,   -- список авторов через запятую
    publisher_name VARCHAR(100),            -- зависит от book_isbn
    publisher_city VARCHAR(50),             -- зависит от publisher_name
    issued_on      DATE NOT NULL,
    due_on         DATE NOT NULL,
    returned_on    DATE
);

CREATE TABLE book_copies (
    book_isbn      VARCHAR(17),
    copy_no        INTEGER,
    book_title     VARCHAR(200),            -- зависит только от book_isbn
    shelf          VARCHAR(10),
    PRIMARY KEY (book_isbn, copy_no)
);
`,

	examRules: `# Требования к экзамену по курсу «Базы данных»

- Допуск: сданы лабораторные работы 1–3.
- Экзамен — тест на dl.spbstu.ru: 20 вопросов, 60 минут, одна попытка.
- Темы: реляционная модель, SQL, нормализация, транзакции, индексы.
`,

	reportSample: `# Пример оформления отчёта по ЛР 3

**Титульный лист**: дисциплина, номер и название работы, группа, ФИО, преподаватель.

## 1. Функциональные зависимости
Таблица loans:
- reader_card → reader_name
- book_isbn → book_title, publisher_name
- publisher_name → publisher_city

## 2. Нормализация
Для каждого шага: нарушенная нормальная форма → устраняемая ФЗ → получившиеся таблицы.

## 3. ER-диаграмма
## 4. DDL-скрипт
## 5. Выводы
`,
}
