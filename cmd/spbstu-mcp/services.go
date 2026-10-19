package main

import (
	"context"

	"github.com/kad-xyz/spbstu-mcp/internal/demo"
	"github.com/kad-xyz/spbstu-mcp/internal/ews"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
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
}

type mailService interface {
	List(ctx context.Context, folder string, limit int, unreadOnly bool, query string) ([]ews.Message, error)
	Read(ctx context.Context, id string, markRead bool) (ews.Message, error)
	Attachment(ctx context.Context, attachmentID string) (string, []byte, error)
	Submit(ctx context.Context, d ews.Draft) error
	Delete(ctx context.Context, id string) error
}

type openeduService interface {
	Courses(ctx context.Context, current bool) ([]openedu.Course, error)
	Progress(ctx context.Context, courseID string) (*openedu.Progress, error)
	Dates(ctx context.Context, courseID string) ([]openedu.Date, error)
	Outline(ctx context.Context, courseID string) ([]openedu.Chapter, error)
}

var (
	_ dlService      = (*moodle.Client)(nil)
	_ mailService    = (*ews.Client)(nil)
	_ openeduService = (*openedu.Client)(nil)

	_ dlService      = demo.DL{}
	_ mailService    = demo.Mail{}
	_ openeduService = demo.OpenEdu{}
)
