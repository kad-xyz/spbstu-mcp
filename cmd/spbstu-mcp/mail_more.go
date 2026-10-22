package main

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mailMoveIn struct {
	Account string   `json:"account" jsonschema:"the account this acts as: must be the active one, from the «Аккаунт:» line of replies or account_list"`
	IDs     []string `json:"ids" jsonschema:"message ids from mail_list"`
	Folder  string   `json:"folder" jsonschema:"inbox, archive, deleteditems, junkemail, or a folder_id from mail_folders"`
}

type mailMarkIn struct {
	Account string   `json:"account" jsonschema:"the account this acts as: must be the active one, from the «Аккаунт:» line of replies or account_list"`
	IDs     []string `json:"ids" jsonschema:"message ids from mail_list"`
	Read    bool     `json:"read" jsonschema:"true marks read, false marks unread"`
}

type calendarIn struct {
	From string `json:"from,omitempty" jsonschema:"first day, YYYY-MM-DD (default today)"`
	Days int    `json:"days,omitempty" jsonschema:"how many days (default 14)"`
}

func registerMailMore(s *mcp.Server, mail mailService) {
	mcp.AddTool(s, &mcp.Tool{Name: "mail_folders", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		Description: "Folders of the university mailbox with total and unread counts; folder_id works with mail_move."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, any, error) {
			v, err := mail.Folders(ctx)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "mail_move", Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
		Description: "Move messages to another folder (archive, a study folder, junk)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mailMoveIn) (*mcp.CallToolResult, any, error) {
			if err := checkAccount(ctx, in.Account); err != nil {
				return nil, nil, err
			}
			if len(in.IDs) == 0 || in.Folder == "" {
				return nil, nil, errors.New("give message ids and a folder")
			}
			if err := mail.Move(ctx, in.IDs, in.Folder); err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Moved."}}}, nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "mail_mark", Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
		Description: "Mark messages read or unread."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mailMarkIn) (*mcp.CallToolResult, any, error) {
			if err := checkAccount(ctx, in.Account); err != nil {
				return nil, nil, err
			}
			if err := mail.MarkRead(ctx, in.IDs, in.Read); err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Done."}}}, nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "mail_calendar", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		Description: "Events of the university mailbox calendar (meetings, webinars, consultations) for a period."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in calendarIn) (*mcp.CallToolResult, any, error) {
			from := time.Now()
			if in.From != "" {
				d, err := time.ParseInLocation("2006-01-02", in.From, time.Local)
				if err != nil {
					return nil, nil, errors.New("from must be YYYY-MM-DD")
				}
				from = d
			}
			from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local)
			v, err := mail.Calendar(ctx, from, from.AddDate(0, 0, orDefault(in.Days, 14)))
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})
}
