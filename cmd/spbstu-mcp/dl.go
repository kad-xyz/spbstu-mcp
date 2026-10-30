package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type coursesIn struct {
	Site string `json:"site,omitempty" jsonschema:"Moodle host; default dl.spbstu.ru"`
	All  bool   `json:"all,omitempty" jsonschema:"include courses past their end date"`
}

type courseIn struct {
	Site     string `json:"site,omitempty" jsonschema:"Moodle host returned by dl_courses; default dl.spbstu.ru"`
	CourseID int    `json:"course_id" jsonschema:"course id from dl_courses"`
}

type deadlinesIn struct {
	Site      string `json:"site,omitempty" jsonschema:"Moodle host; default dl.spbstu.ru"`
	DaysBack  int    `json:"days_back,omitempty" jsonschema:"also show overdue items up to this many days back (default 30)"`
	DaysAhead int    `json:"days_ahead,omitempty" jsonschema:"how far ahead to look (default 120)"`
}

type assignIn struct {
	Site     string `json:"site,omitempty" jsonschema:"Moodle host; default dl.spbstu.ru"`
	AssignID int    `json:"assign_id" jsonschema:"assign_id from dl_assignments"`
}

type forumIn struct {
	Site    string `json:"site,omitempty" jsonschema:"Moodle host; default dl.spbstu.ru"`
	ForumID int    `json:"forum_id" jsonschema:"instance_id of a forum module from dl_search or dl_course_contents"`
}

type discussionIn struct {
	Site         string `json:"site,omitempty" jsonschema:"Moodle host; default dl.spbstu.ru"`
	DiscussionID int    `json:"discussion_id" jsonschema:"discussion_id from dl_forum_discussions"`
}

type pageIn struct {
	Site     string `json:"site,omitempty" jsonschema:"Moodle host; default dl.spbstu.ru"`
	CourseID int    `json:"course_id" jsonschema:"course id"`
	CMID     int    `json:"cmid" jsonschema:"cmid of a page module from dl_course_contents"`
}

type downloadIn struct {
	Site string `json:"site,omitempty" jsonschema:"Moodle host; default dl.spbstu.ru"`
	URL  string `json:"url" jsonschema:"file url from dl_course_contents, dl_assignments or a forum post"`
}

func registerDL(s *mcp.Server, dl dlService, downloads string) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "dl_courses", Annotations: readOnly,
		Description: "List the user's courses on an SPbPU Moodle site (dl.spbstu.ru by default; current ones unless all is set)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in coursesIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			v, err := dl.Courses(ctx, !in.All)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_course_contents", Annotations: readOnly,
		Description: "Sections of a course with their activities (assign, quiz, forum, page, resource, url ...), descriptions and file links."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in courseIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			v, err := dl.Contents(ctx, in.CourseID)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_deadlines", Annotations: readOnly,
		Description: "Upcoming and recently overdue dated items across all courses: assignment due dates, quiz closings."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in deadlinesIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			back, ahead := in.DaysBack, in.DaysAhead
			if back == 0 {
				back = 30
			}
			if ahead == 0 {
				ahead = 120
			}
			v, err := dl.Deadlines(ctx, back, ahead)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_assignments", Annotations: readOnly,
		Description: "Assignments of a course with their full task text, due dates and attached files."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in courseIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			v, err := dl.Assignments(ctx, in.CourseID)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_submission_status", Annotations: readOnly,
		Description: "The user's submission for an assignment: status, submitted files, grade and teacher feedback."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in assignIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			v, err := dl.SubmissionStatus(ctx, in.AssignID)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_grades", Annotations: readOnly,
		Description: "The user's grades in a course."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in courseIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			v, err := dl.Grades(ctx, in.CourseID)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_forum_discussions", Annotations: readOnly,
		Description: "Discussions of a forum (announcements, upload forums, Q&A)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in forumIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			v, err := dl.Discussions(ctx, in.ForumID)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_discussion_posts", Annotations: readOnly,
		Description: "All posts of a forum discussion, oldest first, with attachments."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in discussionIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			v, err := dl.Posts(ctx, in.DiscussionID)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_page", Annotations: readOnly,
		Description: "Text of a page resource (exam rules, ticket lists, instructions)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in pageIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			v, err := dl.Page(ctx, in.CourseID, in.CMID)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: v}}}, nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_download",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
		Description: "Download a file from the selected SPbPU Moodle site (lecture, task, attachment) to the local downloads folder and return its path."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in downloadIn) (*mcp.CallToolResult, any, error) {
			ctx, err := withMoodleSite(ctx, in.Site)
			if err != nil {
				return nil, nil, err
			}
			path, err := dl.Download(ctx, in.URL, downloads)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: path}}}, nil, nil
		})
}
