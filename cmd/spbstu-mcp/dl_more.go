package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type limitIn struct {
	Limit int `json:"limit,omitempty" jsonschema:"how many (default 20)"`
}

type conversationIn struct {
	ConversationID int `json:"conversation_id" jsonschema:"conversation_id from dl_messages"`
}

type sendMessageIn struct {
	Account string `json:"account" jsonschema:"the account this acts as: must be the active one, from the «Аккаунт:» line of replies or account_list"`
	UserID  int    `json:"user_id" jsonschema:"recipient's user_id, from dl_teachers or dl_messages"`
	Text    string `json:"text" jsonschema:"plain-text message"`
}

type searchIn struct {
	Query string `json:"query" jsonschema:"words to find in activity names, descriptions and file names"`
	All   bool   `json:"all,omitempty" jsonschema:"also search courses that have ended"`
}

func orDefault(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}

func registerDLMore(s *mcp.Server, dl dlService) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "dl_quizzes", Annotations: readOnly,
		Description: "Quizzes of a course: when they open and close, time limit, attempts allowed and used, best grade. " +
			"Information only: the server cannot take quizzes."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in courseIn) (*mcp.CallToolResult, any, error) {
			v, err := dl.Quizzes(ctx, in.CourseID)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_teachers", Annotations: readOnly,
		Description: "Teachers of a course with their last visit; user_id is what dl_send_message takes."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in courseIn) (*mcp.CallToolResult, any, error) {
			v, err := dl.Teachers(ctx, in.CourseID)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_notifications", Annotations: readOnly,
		Description: "Newest dl.spbstu.ru notifications (grades, feedback, forum posts, due dates) and the unread count."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in limitIn) (*mcp.CallToolResult, any, error) {
			v, unread, err := dl.Notifications(ctx, orDefault(in.Limit, 20))
			if err != nil {
				return nil, nil, err
			}
			return result(map[string]any{"unread": unread, "notifications": v})
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_messages", Annotations: readOnly,
		Description: "Private conversations on dl.spbstu.ru with their last message and unread count."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in limitIn) (*mcp.CallToolResult, any, error) {
			v, err := dl.Conversations(ctx, orDefault(in.Limit, 20))
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_conversation", Annotations: readOnly,
		Description: "All messages of a dl.spbstu.ru conversation, oldest first."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in conversationIn) (*mcp.CallToolResult, any, error) {
			v, err := dl.ConversationMessages(ctx, in.ConversationID)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_send_message",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(true)},
		Description: "Send a private message on dl.spbstu.ru, e.g. to a course teacher." + confirmFirst},
		func(ctx context.Context, _ *mcp.CallToolRequest, in sendMessageIn) (*mcp.CallToolResult, any, error) {
			if err := checkAccount(ctx, in.Account); err != nil {
				return nil, nil, err
			}
			if err := dl.SendMessage(ctx, in.UserID, in.Text); err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Sent."}}}, nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "dl_search", Annotations: readOnly,
		Description: "Search every current course for activities, resources and files whose name, description or file name contains all the words. Each hit carries the activity's cmid and instance_id (forum tools take the instance_id)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
			v, err := dl.Search(ctx, in.Query, in.All)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})
}
