package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/ews"
)

type mailListIn struct {
	Folder     string `json:"folder,omitempty" jsonschema:"inbox (default), sentitems, drafts, deleteditems or junkemail"`
	Limit      int    `json:"limit,omitempty" jsonschema:"how many of the newest messages (default 20, max 100)"`
	UnreadOnly bool   `json:"unread_only,omitempty" jsonschema:"only unread messages"`
	Query      string `json:"query,omitempty" jsonschema:"search, Exchange AQS syntax: plain words, from:name, subject:word, hasattachment:true"`
}

type mailReadIn struct {
	ID       string `json:"id" jsonschema:"message id from mail_list"`
	MarkRead bool   `json:"mark_read,omitempty" jsonschema:"mark the message as read"`
}

type mailAttachmentIn struct {
	AttachmentID string `json:"attachment_id" jsonschema:"attachment_id from mail_read"`
}

type mailDeleteIn struct {
	ID string `json:"id" jsonschema:"message id from mail_list"`
}

type mailSendIn struct {
	To        []string `json:"to,omitempty" jsonschema:"recipient addresses; for a reply, defaults to the original sender"`
	Cc        []string `json:"cc,omitempty" jsonschema:"copy addresses"`
	Subject   string   `json:"subject,omitempty" jsonschema:"subject; for a reply, defaults to RE: of the original"`
	Body      string   `json:"body" jsonschema:"plain-text body"`
	ReplyToID string   `json:"reply_to_id,omitempty" jsonschema:"id of the message to reply to; the original is quoted below the body"`
	ReplyAll  bool     `json:"reply_all,omitempty" jsonschema:"reply to everyone on the original"`
	Send      bool     `json:"send,omitempty" jsonschema:"true sends the message; false (default) only saves it in Drafts"`
}

func registerMail(s *mcp.Server, mail mailService, downloads string) {
	registerMailDelete(s, mail)
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "mail_list", Annotations: readOnly,
		Description: "List or search the newest messages of the university mailbox (mail.spbstu.ru), newest first."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mailListIn) (*mcp.CallToolResult, any, error) {
			limit := in.Limit
			if limit <= 0 {
				limit = 20
			}
			limit = min(limit, 100)
			v, err := mail.List(ctx, in.Folder, limit, in.UnreadOnly, in.Query)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "mail_read", Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
		Description: "Read a whole message as plain text, with its recipients and attachment list."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mailReadIn) (*mcp.CallToolResult, any, error) {
			v, err := mail.Read(ctx, in.ID, in.MarkRead)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "mail_download_attachment", Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
		Description: "Save a mail attachment to the local downloads folder and return its path."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mailAttachmentIn) (*mcp.CallToolResult, any, error) {
			name, content, err := mail.Attachment(ctx, in.AttachmentID)
			if err != nil {
				return nil, nil, err
			}
			dir := filepath.Join(downloads, "mail")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, err
			}
			path := filepath.Join(dir, filepath.Base(name))
			if err := os.WriteFile(path, content, 0o644); err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: path}}}, nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "mail_send",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(true)},
		Description: "Write a new message or a reply from the university mailbox. By default it is only saved in Drafts. " +
			"Set send=true ONLY after showing the user the exact recipients, subject and body and getting their explicit confirmation."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mailSendIn) (*mcp.CallToolResult, any, error) {
			err := mail.Submit(ctx, ews.Draft{To: in.To, Cc: in.Cc, Subject: in.Subject, Body: in.Body,
				ReplyToID: in.ReplyToID, ReplyAll: in.ReplyAll, Send: in.Send})
			if err != nil {
				return nil, nil, err
			}
			msg := "Saved in Drafts."
			if in.Send {
				msg = "Sent."
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, nil, nil
		})
}

func ptr[T any](v T) *T { return &v }

func registerMailDelete(s *mcp.Server, mail mailService) {
	mcp.AddTool(s, &mcp.Tool{Name: "mail_delete",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)},
		Description: "Move a message (or a draft) to Deleted Items; it can be restored from there. Only after the user's confirmation."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mailDeleteIn) (*mcp.CallToolResult, any, error) {
			if err := mail.Delete(ctx, in.ID); err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Moved to Deleted Items."}}}, nil, nil
		})
}
