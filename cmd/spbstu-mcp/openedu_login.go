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
	Account string `json:"account" jsonschema:"the account this acts as: must be the active one, from the «Аккаунт:» line of replies or account_list"`
	Email   string `json:"email,omitempty" jsonschema:"email of the openedu.ru account, as typed on the openedu.ru sign-in page; leave out to sign in through «Политех» again"`
}

// oePasswords is the credential store as openedu_login and account_remove use it for
// openedu.ru passwords; tests replace it.
var oePasswords = struct {
	load   func(service, email string) (keychain.Credentials, error)
	ask    func(site, email string) (string, error)
	save   func(service, email, password string) error
	remove func(service, email string) error
}{keychain.Load, keychain.AskPasswordFor, keychain.Save, keychain.Delete}

// registerOpenEduLogin adds the tool that picks the openedu.ru account of the active
// university account. It lives apart from the openedu tools because it stores a password.
func registerOpenEduLogin(s *mcp.Server, a *accountSet, oe openeduService) {
	mcp.AddTool(s, &mcp.Tool{Name: "openedu_login",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(true)},
		Description: "Choose how the server signs in to openedu.ru for the active account. openedu.ru keeps two separate accounts " +
			"for one person, one reached through «Политех» (the default here) and one signed in to with email and password, and " +
			"a course enrolled in one does not show up in the other. With email, a system dialog on macOS asks the user for that " +
			"account's openedu.ru password, which goes straight to the keychain, never through the chat; one sign-in checks it, " +
			"and the openedu_* tools of this account then read and write as that openedu.ru account, also after a restart. " +
			"Without email, back to «Политех»; the openedu.ru password of the email left behind is removed from the keychain. " +
			"Use it when the user asks, e.g. after openedu_courses misses courses the user sees on openedu.ru."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in oeLoginIn) (*mcp.CallToolResult, any, error) {
			if a.demo {
				return nil, nil, errDemo
			}
			if err := checkAccount(ctx, in.Account); err != nil {
				return nil, nil, err
			}
			service := openedu.PasswordService(a.service)
			// The email the account signed in with so far: its password has no owner once
			// the account signs in another way.
			left := openedu.Login(a.reg.Store(sess(ctx).account))
			email := strings.ToLower(strings.TrimSpace(in.Email))
			if email == "" {
				user, err := oe.SignInAs(ctx, "")
				if err != nil {
					return nil, nil, fmt.Errorf("openedu.ru sign-in through «Политех» failed, the server signs in as before: %w", err)
				}
				if left != "" {
					_ = oePasswords.remove(service, left)
				}
				return text("openedu.ru: signed in through «Политех» as user " + user + ".")
			}
			if !strings.Contains(email, "@") {
				return nil, nil, errors.New("give the email of the openedu.ru account")
			}
			if _, err := oePasswords.load(service, email); err != nil {
				password, err := oePasswords.ask("openedu.ru", email)
				if errors.Is(err, keychain.ErrUnsupported) {
					return text("Store the openedu.ru password first: run `" + keychain.StoreCommand(service, email) +
						"` in a terminal, type the password of the openedu.ru account (not the university one) when asked, " +
						"then call openedu_login again.")
				}
				if err != nil {
					return nil, nil, err
				}
				if err := oePasswords.save(service, email, password); err != nil {
					return nil, nil, err
				}
			}
			user, err := oe.SignInAs(ctx, email)
			switch {
			case errors.Is(err, openedu.ErrRejected):
				// Only a password openedu.ru called wrong is dropped, so that the next call asks
				// for it again. After any other failure the password may be right, and typing
				// it again would only add a sign-in.
				_ = oePasswords.remove(service, email)
				return nil, nil, fmt.Errorf("openedu.ru sign-in as %s failed, the server signs in as before. The stored password was "+
					"removed: call openedu_login again to type it anew: %w", email, err)
			case errors.Is(err, openedu.ErrLocked):
				return nil, nil, fmt.Errorf("openedu.ru sign-in as %s failed, the server signs in as before. The password is kept: "+
					"wait before calling openedu_login again, every attempt now prolongs the block: %w", email, err)
			case err != nil:
				return nil, nil, fmt.Errorf("openedu.ru sign-in as %s failed, the server signs in as before. The password is kept, "+
					"openedu_login can be called again: %w", email, err)
			}
			if left != "" && left != email {
				_ = oePasswords.remove(service, left)
			}
			return text("openedu.ru: signed in as " + email + " (user " + user + "); the openedu_* tools now use this account.")
		})
}
