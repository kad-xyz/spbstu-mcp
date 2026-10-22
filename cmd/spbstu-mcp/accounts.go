package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/accounts"
	"github.com/kad-xyz/spbstu-mcp/internal/demo"
	"github.com/kad-xyz/spbstu-mcp/internal/ews"
	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
	"github.com/kad-xyz/spbstu-mcp/internal/mypoly"
	"github.com/kad-xyz/spbstu-mcp/internal/openedu"
)

const demoAccount = "ivanov.ii@edu.spbstu.ru"

// session is everything the tools use on behalf of one account. A tool call takes one
// session for its whole run, so its reads, writes and the account line of its reply all
// belong to the same person even if the account is switched meanwhile.
type session struct {
	account string
	dl      dlService
	oe      openeduService
	mail    mailService
	my      myService
}

// accountSet keeps a session per account and knows which account is active.
type accountSet struct {
	reg     *accounts.Registry
	service string // credential store service holding the passwords
	demo    bool

	mu       sync.Mutex
	sessions map[string]*session
}

func newAccountSet(reg *accounts.Registry, service string, demoMode bool) *accountSet {
	return &accountSet{reg: reg, service: service, demo: demoMode, sessions: map[string]*session{}}
}

// active returns the session of the active account.
func (a *accountSet) active() (*session, error) {
	if a.demo {
		return &session{account: demoAccount, dl: demo.DL{}, oe: demo.OpenEdu{}, mail: demo.Mail{}, my: demo.My{}}, nil
	}
	account, err := a.reg.Active()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.session(account), nil
}

// session returns the session of an added account, created on first use; a.mu is held.
func (a *accountSet) session(account string) *session {
	if s := a.sessions[account]; s != nil {
		return s
	}
	store := a.reg.Store(account)
	s := &session{
		account: account,
		dl:      &moodle.Client{KeychainService: a.service, KeychainAccount: account, State: store},
		oe:      &openedu.Client{KeychainService: a.service, KeychainAccount: account, State: store},
		mail:    &ews.Client{KeychainService: a.service, KeychainAccount: account},
		my:      &mypoly.Client{KeychainService: a.service, KeychainAccount: account, State: store},
	}
	a.sessions[account] = s
	return s
}

// forget drops the session of a removed account.
func (a *accountSet) forget(account string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, account)
}

type sessionKey struct{}

// sess is the session the middleware chose for this tool call.
func sess(ctx context.Context) *session {
	s, _ := ctx.Value(sessionKey{}).(*session)
	if s == nil {
		panic("tool called without an account session")
	}
	return s
}

// checkAccount refuses a write whose account argument is not the account of the call.
func checkAccount(ctx context.Context, account string) error {
	want := sess(ctx).account
	switch got := keychain.Account(account); {
	case got == "":
		return fmt.Errorf("give account: the active account is %s", want)
	case got != want:
		return fmt.Errorf("refused: account is %s, but the active account is %s. Nothing was done. "+
			"Switch with account_switch, or act as %s only if the user means it", got, want, want)
	}
	return nil
}

// accountLine starts every reply, so that the user and the model see whose data it is.
func accountLine(account string) mcp.Content {
	return &mcp.TextContent{Text: "Аккаунт: " + account}
}

// middleware runs every tool call with the session of the active account and prefixes its
// reply with that account. account_* tools manage the accounts themselves and are prefixed
// with the account active after them.
func (a *accountSet) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		call, ok := req.(*mcp.CallToolRequest)
		if !ok || method != "tools/call" {
			return next(ctx, method, req)
		}
		var account string
		if !strings.HasPrefix(call.Params.Name, "account_") {
			s, err := a.active()
			if err != nil {
				res := &mcp.CallToolResult{}
				res.SetError(err)
				return res, nil
			}
			ctx, account = context.WithValue(ctx, sessionKey{}, s), s.account
		}
		res, err := next(ctx, method, req)
		if account == "" {
			if s, err := a.active(); err == nil {
				account = s.account
			}
		}
		if r, ok := res.(*mcp.CallToolResult); ok && r != nil && account != "" {
			r.Content = append([]mcp.Content{accountLine(account)}, r.Content...)
		}
		return res, err
	}
}

// The proxies hand every call to the service of the call's session.

type dlProxy struct{}

func (dlProxy) Courses(ctx context.Context, current bool) ([]moodle.Course, error) {
	return sess(ctx).dl.Courses(ctx, current)
}
func (dlProxy) Contents(ctx context.Context, courseID int) ([]moodle.Section, error) {
	return sess(ctx).dl.Contents(ctx, courseID)
}
func (dlProxy) Deadlines(ctx context.Context, daysBack, daysAhead int) ([]moodle.Deadline, error) {
	return sess(ctx).dl.Deadlines(ctx, daysBack, daysAhead)
}
func (dlProxy) Assignments(ctx context.Context, courseID int) ([]moodle.Assignment, error) {
	return sess(ctx).dl.Assignments(ctx, courseID)
}
func (dlProxy) SubmissionStatus(ctx context.Context, assignID int) (map[string]any, error) {
	return sess(ctx).dl.SubmissionStatus(ctx, assignID)
}
func (dlProxy) Grades(ctx context.Context, courseID int) ([]moodle.GradeItem, error) {
	return sess(ctx).dl.Grades(ctx, courseID)
}
func (dlProxy) Discussions(ctx context.Context, forumID int) ([]moodle.Discussion, error) {
	return sess(ctx).dl.Discussions(ctx, forumID)
}
func (dlProxy) Posts(ctx context.Context, discussionID int) ([]moodle.Post, error) {
	return sess(ctx).dl.Posts(ctx, discussionID)
}
func (dlProxy) Page(ctx context.Context, courseID, cmid int) (string, error) {
	return sess(ctx).dl.Page(ctx, courseID, cmid)
}
func (dlProxy) Download(ctx context.Context, fileURL, dir string) (string, error) {
	return sess(ctx).dl.Download(ctx, fileURL, dir)
}
func (dlProxy) SaveSubmission(ctx context.Context, assignID int, paths []string, text string, submit bool) error {
	return sess(ctx).dl.SaveSubmission(ctx, assignID, paths, text, submit)
}
func (dlProxy) AddDiscussion(ctx context.Context, forumID int, subject, message string, paths []string) (int, error) {
	return sess(ctx).dl.AddDiscussion(ctx, forumID, subject, message, paths)
}
func (dlProxy) Reply(ctx context.Context, postID int, subject, message string, paths []string) error {
	return sess(ctx).dl.Reply(ctx, postID, subject, message, paths)
}
func (dlProxy) Quizzes(ctx context.Context, courseID int) ([]moodle.Quiz, error) {
	return sess(ctx).dl.Quizzes(ctx, courseID)
}
func (dlProxy) Teachers(ctx context.Context, courseID int) ([]moodle.Person, error) {
	return sess(ctx).dl.Teachers(ctx, courseID)
}
func (dlProxy) Notifications(ctx context.Context, limit int) ([]moodle.Notification, int, error) {
	return sess(ctx).dl.Notifications(ctx, limit)
}
func (dlProxy) Conversations(ctx context.Context, limit int) ([]moodle.Conversation, error) {
	return sess(ctx).dl.Conversations(ctx, limit)
}
func (dlProxy) ConversationMessages(ctx context.Context, conversationID int) ([]moodle.Message, error) {
	return sess(ctx).dl.ConversationMessages(ctx, conversationID)
}
func (dlProxy) SendMessage(ctx context.Context, userID int, text string) error {
	return sess(ctx).dl.SendMessage(ctx, userID, text)
}
func (dlProxy) Search(ctx context.Context, query string, all bool) ([]moodle.Hit, error) {
	return sess(ctx).dl.Search(ctx, query, all)
}

type mailProxy struct{}

func (mailProxy) List(ctx context.Context, folder string, limit int, unreadOnly bool, query string) ([]ews.Message, error) {
	return sess(ctx).mail.List(ctx, folder, limit, unreadOnly, query)
}
func (mailProxy) Read(ctx context.Context, id string, markRead bool) (ews.Message, error) {
	return sess(ctx).mail.Read(ctx, id, markRead)
}
func (mailProxy) Attachment(ctx context.Context, attachmentID string) (string, []byte, error) {
	return sess(ctx).mail.Attachment(ctx, attachmentID)
}
func (mailProxy) Submit(ctx context.Context, d ews.Draft) error { return sess(ctx).mail.Submit(ctx, d) }
func (mailProxy) Delete(ctx context.Context, id string) error   { return sess(ctx).mail.Delete(ctx, id) }
func (mailProxy) Folders(ctx context.Context) ([]ews.Folder, error) {
	return sess(ctx).mail.Folders(ctx)
}
func (mailProxy) Move(ctx context.Context, ids []string, folder string) error {
	return sess(ctx).mail.Move(ctx, ids, folder)
}
func (mailProxy) MarkRead(ctx context.Context, ids []string, read bool) error {
	return sess(ctx).mail.MarkRead(ctx, ids, read)
}
func (mailProxy) Calendar(ctx context.Context, from, to time.Time) ([]ews.Event, error) {
	return sess(ctx).mail.Calendar(ctx, from, to)
}

type openeduProxy struct{}

func (openeduProxy) Courses(ctx context.Context, current bool) ([]openedu.Course, error) {
	return sess(ctx).oe.Courses(ctx, current)
}
func (openeduProxy) Progress(ctx context.Context, courseID string) (*openedu.Progress, error) {
	return sess(ctx).oe.Progress(ctx, courseID)
}
func (openeduProxy) Dates(ctx context.Context, courseID string) ([]openedu.Date, error) {
	return sess(ctx).oe.Dates(ctx, courseID)
}
func (openeduProxy) Outline(ctx context.Context, courseID string) ([]openedu.Chapter, error) {
	return sess(ctx).oe.Outline(ctx, courseID)
}
func (openeduProxy) Units(ctx context.Context, courseID, block string) ([]openedu.Unit, error) {
	return sess(ctx).oe.Units(ctx, courseID, block)
}
func (openeduProxy) Problems(ctx context.Context, courseID, block string) ([]openedu.Problem, error) {
	return sess(ctx).oe.Problems(ctx, courseID, block)
}
func (openeduProxy) Answer(ctx context.Context, courseID, problem string, answers map[string][]string, submit bool) (*openedu.Problem, error) {
	return sess(ctx).oe.Answer(ctx, courseID, problem, answers, submit)
}
func (openeduProxy) Exam(ctx context.Context, courseID, block string, start bool) (*openedu.Exam, error) {
	return sess(ctx).oe.Exam(ctx, courseID, block, start)
}
func (openeduProxy) FinishExam(ctx context.Context, courseID, block string) (*openedu.Exam, error) {
	return sess(ctx).oe.FinishExam(ctx, courseID, block)
}

type myProxy struct{}

func (myProxy) RecordBooks(ctx context.Context) ([]mypoly.RecordBook, error) {
	return sess(ctx).my.RecordBooks(ctx)
}
func (myProxy) Grades(ctx context.Context, recordBook string, semester int) (*mypoly.Grades, error) {
	return sess(ctx).my.Grades(ctx, recordBook, semester)
}
func (myProxy) StudyPlan(ctx context.Context, recordBook string, semester int) ([]mypoly.Discipline, error) {
	return sess(ctx).my.StudyPlan(ctx, recordBook, semester)
}
func (myProxy) WorkPlan(ctx context.Context, recordBook string, semester int) (map[string][]mypoly.Discipline, error) {
	return sess(ctx).my.WorkPlan(ctx, recordBook, semester)
}

var (
	_ dlService      = dlProxy{}
	_ mailService    = mailProxy{}
	_ openeduService = openeduProxy{}
	_ myService      = myProxy{}
)

type accountIn struct {
	Account string `json:"account" jsonschema:"university login, with or without @edu.spbstu.ru, e.g. ivanov.ii"`
}

type accountInfo struct {
	Account string `json:"account"`
	Active  bool   `json:"active,omitempty"`
}

var errDemo = errors.New("accounts cannot be changed in -demo mode")

func registerAccounts(s *mcp.Server, a *accountSet) {
	mcp.AddTool(s, &mcp.Tool{Name: "account_list", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		Description: "The university accounts the server can act as, and which one is active: every other tool works as the active account."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, any, error) {
			cur, err := a.active()
			if err != nil {
				return nil, nil, err
			}
			list := []string{cur.account}
			if !a.demo {
				if list, err = a.reg.List(); err != nil {
					return nil, nil, err
				}
			}
			out := make([]accountInfo, len(list))
			for i, acc := range list {
				out[i] = accountInfo{Account: acc, Active: acc == cur.account}
			}
			return result(out)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "account_switch", Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
		Description: "Make another added account the active one, e.g. to help a classmate and then come back. Every tool then reads " +
			"and writes as that account, also after a restart, until switched back. Switch only when the user asks for it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in accountIn) (*mcp.CallToolResult, any, error) {
			if a.demo {
				return nil, nil, errDemo
			}
			account := keychain.Account(in.Account)
			if account == "" {
				return nil, nil, errors.New("give account")
			}
			if !a.reg.Has(account) {
				return nil, nil, fmt.Errorf("account %s is not added: add it with account_add", account)
			}
			if err := a.reg.SetActive(account); err != nil {
				return nil, nil, err
			}
			return text("Active account: " + account + ".")
		})

	mcp.AddTool(s, &mcp.Tool{Name: "account_add",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(true)},
		Description: "Add a university account, e.g. a classmate's. On macOS a system dialog asks the user for its password, which goes " +
			"straight to the keychain, never through the chat; then one sign-in checks it. Elsewhere it returns the command that stores " +
			"the password, to run before calling it again. The active account stays as it was."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in accountIn) (*mcp.CallToolResult, any, error) {
			if a.demo {
				return nil, nil, errDemo
			}
			account := keychain.Account(in.Account)
			if account == "" {
				return nil, nil, errors.New("give account")
			}
			if _, err := keychain.Load(a.service, account); err == nil && a.reg.Has(account) {
				return text("Account " + account + " is already added.")
			}
			saved := false
			if _, err := keychain.Load(a.service, account); err != nil {
				password, err := keychain.AskPassword(account)
				if errors.Is(err, keychain.ErrUnsupported) {
					return text("Store the password first: run `" + keychain.StoreCommand(a.service, account) +
						"` in a terminal, type the password when asked, then call account_add again.")
				}
				if err != nil {
					return nil, nil, err
				}
				if err := keychain.Save(a.service, account, password); err != nil {
					return nil, nil, err
				}
				saved = true
			}
			if err := a.reg.Add(account); err != nil {
				return nil, nil, err
			}
			// One sign-in to dl.spbstu.ru checks the password; its token stays in the new store.
			a.mu.Lock()
			check := a.session(account)
			a.mu.Unlock()
			if _, err := check.dl.Courses(ctx, true); err != nil {
				a.forget(account)
				_ = a.reg.Remove(account)
				if saved {
					_ = keychain.Delete(a.service, account)
				}
				return nil, nil, fmt.Errorf("sign-in as %s failed, the account was not added: %w", account, err)
			}
			return text("Added " + account + ". Switch to it with account_switch.")
		})

	mcp.AddTool(s, &mcp.Tool{Name: "account_remove", Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)},
		Description: "Remove an account that is not active: its password from the keychain and its saved sessions. Only after the user's confirmation."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in accountIn) (*mcp.CallToolResult, any, error) {
			if a.demo {
				return nil, nil, errDemo
			}
			account := keychain.Account(in.Account)
			if account == "" {
				return nil, nil, errors.New("give account")
			}
			if err := a.reg.Remove(account); err != nil {
				return nil, nil, err
			}
			a.forget(account)
			msg := "Removed " + account + "."
			if err := keychain.Delete(a.service, account); err != nil {
				msg += " Its password is still stored: " + err.Error()
			}
			return text(msg)
		})
}

func text(s string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}, nil, nil
}
