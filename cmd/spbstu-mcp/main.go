// Command spbstu-mcp is an MCP server for the SPbPU student services: the dl.spbstu.ru Moodle
// and my.spbstu.ru. It signs in with logins kept in the system credential store and acts as
// one of them at a time, the active account.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/accounts"
	"github.com/kad-xyz/spbstu-mcp/internal/answers"
	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
)

var version = "dev"

func main() {
	service := flag.String("keychain-service", "spbstu", "generic password item holding the SPbPU login")
	home, _ := os.UserHomeDir()
	demoMode := flag.Bool("demo", false, "serve made-up data instead of the university services (no account needed)")
	group := flag.String("group", "", "default group for the schedule tool, e.g. 5130902/30201")
	downloads := flag.String("downloads", filepath.Join(home, "Downloads", "spbstu"), "where dl_download saves files")
	flag.Parse()

	reg := accounts.Default(func() (string, error) {
		c, err := keychain.Load(*service, "")
		return c.Account, err
	})
	accts := newAccountSet(reg, *service, *demoMode)

	server := mcp.NewServer(&mcp.Implementation{Name: "spbstu-mcp", Version: version}, &mcp.ServerOptions{
		Instructions: "SPbPU (Политех) student services of the user. For \"what is due / what is going on\" start with study_overview. dl_* tools read the dl.spbstu.ru Moodle: " +
			"start with dl_courses, then dl_course_contents / dl_assignments / dl_deadlines. Forum tools take the " +
			"forum's instance_id from dl_course_contents, not its cmid. mail_* tools work with the university " +
			"mailbox on mail.spbstu.ru; mail_send saves a draft unless send=true, which needs the user's explicit confirmation. " +
			"openedu_* tools read the user's openedu.ru courses (progress, dates, outline, page text); openedu_problems reads a test's questions and openedu_answer saves or submits the user's answers (submitting spends an attempt); timed exams are started and read with openedu_exam and finished with openedu_exam_finish; openedu_enroll enrolls in a course run. schedule reads the public timetable. my_* tools read my.spbstu.ru: grades (record book), study and work plans, debts. For one discipline use study_subject. answers_* tools search the student answer bases polyvsp.ru and polyphis.ru." + accountRules + answersRules,
	})
	server.AddReceivingMiddleware(accts.middleware)
	var (
		dlSvc   dlService      = dlProxy{}
		oeSvc   openeduService = openeduProxy{}
		mailSvc mailService    = mailProxy{}
		mySvc   myService      = myProxy{}
	)
	registerAccounts(server, accts)
	registerDL(server, dlSvc, *downloads)
	registerDLWrite(server, dlSvc)
	registerMail(server, mailSvc, *downloads)
	registerOpenEdu(server, oeSvc)
	registerOverview(server, dlSvc, oeSvc, mailSvc)
	registerSchedule(server, *group)
	registerAnswers(server, answers.Default())
	registerMy(server, mySvc)
	registerDLMore(server, dlSvc)
	registerMailMore(server, mailSvc)
	registerStudy(server, dlSvc, oeSvc, mySvc, *downloads)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

// accountRules keep a classmate's account from being used by mistake.
const accountRules = " The server can hold several university accounts (the user's own and, say, a classmate's) and acts as the active one: " +
	"every reply starts with «Аккаунт: …», the account it was made as. account_list shows them, account_switch changes the active one " +
	"(only when the user asks), account_add adds one (the password is typed into a system dialog, never into the chat). " +
	"Tools that change something take account, which must be the active account, or they refuse; take it from the «Аккаунт:» line, " +
	"and before acting as anyone but the user's own account, name that account to the user."

// result renders any value as indented JSON text for the model.
func result(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}
