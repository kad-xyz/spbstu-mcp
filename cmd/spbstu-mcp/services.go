package main

import (
	"context"
	"time"

	"github.com/kad-xyz/spbstu-mcp/internal/demo"
	"github.com/kad-xyz/spbstu-mcp/internal/ews"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
	"github.com/kad-xyz/spbstu-mcp/internal/mypoly"
	"github.com/kad-xyz/spbstu-mcp/internal/openedu"
)

// The tools depend on these interfaces, so that -demo can serve made-up data.

type dlService interface {
	Courses(ctx context.Context, current bool) ([]moodle.Course, error)
	Contents(ctx context.Context, courseID int) ([]moodle.Section, error)
	Deadlines(ctx context.Context, daysBack, daysAhead int) ([]moodle.Deadline, error)
	Assignments(ctx context.Context, courseID int) ([]moodle.Assignment, error)
	SubmissionStatus(ctx context.Context, assignID int) (map[string]any, error)
	Grades(ctx context.Context, courseID int) ([]moodle.GradeItem, error)
	Discussions(ctx context.Context, forumID int) ([]moodle.Discussion, error)
	Posts(ctx context.Context, discussionID int) ([]moodle.Post, error)
	Page(ctx context.Context, courseID, cmid int) (string, error)
	Download(ctx context.Context, fileURL, dir string) (string, error)
	SaveSubmission(ctx context.Context, assignID int, paths []string, text string, submit bool) error
	AddDiscussion(ctx context.Context, forumID int, subject, message string, paths []string) (int, error)
	Reply(ctx context.Context, postID int, subject, message string, paths []string) error
	Quizzes(ctx context.Context, courseID int) ([]moodle.Quiz, error)
	Teachers(ctx context.Context, courseID int) ([]moodle.Person, error)
	Notifications(ctx context.Context, limit int) ([]moodle.Notification, int, error)
	Conversations(ctx context.Context, limit int) ([]moodle.Conversation, error)
	ConversationMessages(ctx context.Context, conversationID int) ([]moodle.Message, error)
	SendMessage(ctx context.Context, userID int, text string) error
	Search(ctx context.Context, query string, all bool) ([]moodle.Hit, error)
}

type mailService interface {
	List(ctx context.Context, folder string, limit int, unreadOnly bool, query string) ([]ews.Message, error)
	Read(ctx context.Context, id string, markRead bool) (ews.Message, error)
	Attachment(ctx context.Context, attachmentID string) (string, []byte, error)
	Submit(ctx context.Context, d ews.Draft) error
	Delete(ctx context.Context, id string) error
	Folders(ctx context.Context) ([]ews.Folder, error)
	Move(ctx context.Context, ids []string, folder string) error
	MarkRead(ctx context.Context, ids []string, read bool) error
	Calendar(ctx context.Context, from, to time.Time) ([]ews.Event, error)
}

type openeduService interface {
	Courses(ctx context.Context, current bool) ([]openedu.Course, error)
	Progress(ctx context.Context, courseID string) (*openedu.Progress, error)
	Dates(ctx context.Context, courseID string) ([]openedu.Date, error)
	Outline(ctx context.Context, courseID string) ([]openedu.Chapter, error)
	Units(ctx context.Context, courseID, block string) ([]openedu.Unit, error)
	Problems(ctx context.Context, courseID, block string) ([]openedu.Problem, error)
	Answer(ctx context.Context, courseID, problem string, answers map[string][]string, submit bool) (*openedu.Problem, error)
	Exam(ctx context.Context, courseID, block string, start bool) (*openedu.Exam, error)
	FinishExam(ctx context.Context, courseID, block string) (*openedu.Exam, error)
}

type myService interface {
	RecordBooks(ctx context.Context) ([]mypoly.RecordBook, error)
	Grades(ctx context.Context, recordBook string, semester int) (*mypoly.Grades, error)
	StudyPlan(ctx context.Context, recordBook string, semester int) ([]mypoly.Discipline, error)
	WorkPlan(ctx context.Context, recordBook string, semester int) (map[string][]mypoly.Discipline, error)
}

var (
	_ myService      = (*mypoly.Client)(nil)
	_ myService      = demo.My{}
	_ dlService      = (*moodle.Client)(nil)
	_ mailService    = (*ews.Client)(nil)
	_ openeduService = (*openedu.Client)(nil)

	_ dlService      = demo.DL{}
	_ mailService    = demo.Mail{}
	_ openeduService = demo.OpenEdu{}
)
