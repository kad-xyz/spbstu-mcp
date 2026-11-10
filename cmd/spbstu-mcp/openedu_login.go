package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
	"github.com/kad-xyz/spbstu-mcp/internal/openedu"
)

type oeLoginIn struct {
	Email string `json:"email,omitempty" jsonschema:"email of the openedu.ru account, as typed on the openedu.ru sign-in page; leave out to sign in through «Политех» again"`
}

// registerOpenEduLogin adds the tool that picks the openedu.ru account of the active
// university account. It lives apart from the openedu tools because it stores a password.
func registerOpenEduLogin(s *mcp.Server, a *accountSet, oe openeduService) {
	mcp.AddTool(s, &mcp.Tool{Name: "openedu_login",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(true)},
		Description: "Choose how the server signs in to openedu.ru for the active account. openedu.ru keeps two separate accounts " +
			"for one person, one reached through «Политех» (the default here) and one signed in to with email and password, and " +
			"a course enrolled in one does not show up in the other. With email, a system dialog on macOS asks the user for that " +
			"account's openedu.ru password, which goes straight to the keychain, never through the chat; one sign-in checks it, " +
			"and the openedu_* tools then use that account, also after a restart. Without email, back to «Политех». Use it when " +
			"the user asks, e.g. after openedu_courses misses courses the user sees on openedu.ru."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in oeLoginIn) (*mcp.CallToolResult, any, error) {
			if a.demo {
				return nil, nil, errors.New("the openedu.ru account cannot be changed in -demo mode")
			}
			email := strings.ToLower(strings.TrimSpace(in.Email))
			if email == "" {
				user, err := oe.SignInAs(ctx, "")
				if err != nil {
					return nil, nil, fmt.Errorf("openedu.ru sign-in through «Политех» failed, the server signs in as before: %w", err)
				}
				return text("openedu.ru: signed in through «Политех» as user " + user + ".")
			}
			if !strings.Contains(email, "@") {
				return nil, nil, errors.New("give the email of the openedu.ru account")
			}
			service := openedu.PasswordService(a.service)
			saved := false
			if _, err := keychain.Load(service, email); err != nil {
				password, err := keychain.AskPassword(email)
				if errors.Is(err, keychain.ErrUnsupported) {
					return text("Store the openedu.ru password first: run `" + keychain.StoreCommand(service, email) +
						"` in a terminal, type the password when asked, then call openedu_login again.")
				}
				if err != nil {
					return nil, nil, err
				}
				if err := keychain.Save(service, email, password); err != nil {
					return nil, nil, err
				}
				saved = true
			}
			user, err := oe.SignInAs(ctx, email)
			if err != nil {
				// A rejected password is dropped, also one stored before, so that the next call asks again.
				if saved || errors.Is(err, openedu.ErrRejected) {
					_ = keychain.Delete(service, email)
				}
				return nil, nil, fmt.Errorf("openedu.ru sign-in as %s failed, the server signs in as before: %w", email, err)
			}
			return text("openedu.ru: signed in as " + email + " (user " + user + "); the openedu_* tools now use this account.")
		})
}
