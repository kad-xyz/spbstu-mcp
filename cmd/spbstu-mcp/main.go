// Command spbstu-mcp is an MCP server for the SPbPU student services: the dl.spbstu.ru Moodle
// and my.spbstu.ru. It signs in with a login kept in the macOS keychain.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/ews"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
	"github.com/kad-xyz/spbstu-mcp/internal/openedu"
	"github.com/kad-xyz/spbstu-mcp/internal/state"
)

var version = "dev"

func main() {
	service := flag.String("keychain-service", "spbstu", "generic password item holding the SPbPU login")
	home, _ := os.UserHomeDir()
	group := flag.String("group", "", "default group for the schedule tool, e.g. 5130902/30201")
	downloads := flag.String("downloads", filepath.Join(home, "Downloads", "spbstu"), "where dl_download saves files")
	flag.Parse()

	store := state.Default()
	dl := &moodle.Client{KeychainService: *service, State: store}

	server := mcp.NewServer(&mcp.Implementation{Name: "spbstu-mcp", Version: version}, &mcp.ServerOptions{
		Instructions: "SPbPU (Политех) student services of the user. For \"what is due / what is going on\" start with study_overview. dl_* tools read the dl.spbstu.ru Moodle: " +
			"start with dl_courses, then dl_course_contents / dl_assignments / dl_deadlines. Forum tools take the " +
			"forum's instance_id from dl_course_contents, not its cmid. mail_* tools work with the university " +
			"mailbox on mail.spbstu.ru; mail_send saves a draft unless send=true, which needs the user's explicit confirmation. " +
			"openedu_* tools read the user's openedu.ru courses (progress, dates, outline). schedule reads the public timetable.",
	})
	oe := &openedu.Client{KeychainService: *service, State: store}
	mail := &ews.Client{KeychainService: *service}
	registerDL(server, dl, *downloads)
	registerDLWrite(server, dl)
	registerMail(server, mail, *downloads)
	registerOpenEdu(server, oe)
	registerOverview(server, dl, oe, mail)
	registerSchedule(server, *group)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

// result renders any value as indented JSON text for the model.
func result(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}
