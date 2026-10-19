package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const confirmFirst = " Act ONLY after showing the user exactly what will be sent where and getting their explicit confirmation."

type submitIn struct {
	AssignID int      `json:"assign_id" jsonschema:"assign_id from dl_assignments"`
	Files    []string `json:"files,omitempty" jsonschema:"absolute paths of local files; they REPLACE every file already in the submission"`
	Text     string   `json:"text,omitempty" jsonschema:"online text answer, for assignments that take one"`
	Submit   bool     `json:"submit_for_grading,omitempty" jsonschema:"also press 'submit for grading' (the submission can no longer be edited); default leaves it as a draft"`
}

type discussionNewIn struct {
	ForumID int      `json:"forum_id" jsonschema:"instance_id of the forum from dl_course_contents"`
	Subject string   `json:"subject" jsonschema:"discussion subject, e.g. the user's full name and group for upload forums"`
	Message string   `json:"message" jsonschema:"message text (HTML allowed)"`
	Files   []string `json:"files,omitempty" jsonschema:"absolute paths of local files to attach"`
}

type replyIn struct {
	PostID  int      `json:"post_id" jsonschema:"post_id from dl_discussion_posts"`
	Subject string   `json:"subject" jsonschema:"reply subject, usually Re: of the post"`
	Message string   `json:"message" jsonschema:"message text (HTML allowed)"`
	Files   []string `json:"files,omitempty" jsonschema:"absolute paths of local files to attach"`
}

func registerDLWrite(s *mcp.Server, dl dlService) {
	write := &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(true)}

	mcp.AddTool(s, &mcp.Tool{Name: "dl_submit_assignment", Annotations: write,
		Description: "Upload the user's work to an assignment on dl.spbstu.ru: files and/or online text, saved as a draft " +
			"submission, optionally submitted for grading. New files replace the files already submitted." + confirmFirst},
		func(ctx context.Context, _ *mcp.CallToolRequest, in submitIn) (*mcp.CallToolResult, any, error) {
			if err := dl.SaveSubmission(ctx, in.AssignID, in.Files, in.Text, in.Submit); err != nil {
				return nil, nil, err
			}
			status, err := dl.SubmissionStatus(ctx, in.AssignID)
			if err != nil {
				return nil, nil, err
			}
			return result(status)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_forum_new_discussion", Annotations: write,
		Description: "Start a discussion in a forum on dl.spbstu.ru, e.g. post a report to an upload forum or create the " +
			"exam identification topic, with optional attachments." + confirmFirst},
		func(ctx context.Context, _ *mcp.CallToolRequest, in discussionNewIn) (*mcp.CallToolResult, any, error) {
			id, err := dl.AddDiscussion(ctx, in.ForumID, in.Subject, in.Message, in.Files)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Posted, discussion_id %d.", id)}}}, nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_forum_reply", Annotations: write,
		Description: "Reply to a forum post on dl.spbstu.ru, with optional attachments." + confirmFirst},
		func(ctx context.Context, _ *mcp.CallToolRequest, in replyIn) (*mcp.CallToolResult, any, error) {
			if err := dl.Reply(ctx, in.PostID, in.Subject, in.Message, in.Files); err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Posted."}}}, nil, nil
		})
}
