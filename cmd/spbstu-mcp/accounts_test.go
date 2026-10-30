package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/accounts"
	"github.com/kad-xyz/spbstu-mcp/internal/demo"
	"github.com/kad-xyz/spbstu-mcp/internal/ews"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
)

// draftMail saves drafts, where demo.Mail refuses every write.
type draftMail struct{ demo.Mail }

func (draftMail) Submit(context.Context, ews.Draft) error { return nil }

const (
	own      = "ivanov.ii@edu.spbstu.ru"
	classmat = "petrov.pp@edu.spbstu.ru"
)

// connect serves the tools over an in-memory transport, with made-up services for two
// added accounts, the first of them active.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	reg := &accounts.Registry{Dir: t.TempDir(), First: func() (string, error) { return own, nil }}
	if err := reg.Add(classmat); err != nil {
		t.Fatal(err)
	}
	a := newAccountSet(reg, "spbstu-mcp-test", false)
	for _, acc := range []string{own, classmat} {
		a.sessions[acc] = &session{account: acc, dl: map[string]dlService{moodle.DefaultSite: demo.DL{}}, oe: demo.OpenEdu{}, mail: draftMail{}, my: demo.My{}}
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	s.AddReceivingMiddleware(a.middleware)
	registerAccounts(s, a)
	registerMail(s, mailProxy{}, t.TempDir())
	registerMailMore(s, mailProxy{})

	ct, st := mcp.NewInMemoryTransports()
	if _, err := s.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	c, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// call returns the text of a reply, its first line apart, and whether it is an error.
func call(t *testing.T, c *mcp.ClientSession, tool string, args map[string]any) (first, rest string, isErr bool) {
	t.Helper()
	res, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, ct := range res.Content {
		parts = append(parts, ct.(*mcp.TextContent).Text)
	}
	if len(parts) == 0 {
		t.Fatalf("%s: empty reply", tool)
	}
	return parts[0], strings.Join(parts[1:], "\n"), res.IsError
}

func TestWritesNeedTheActiveAccount(t *testing.T) {
	c := connect(t)
	send := map[string]any{"body": "hi", "to": []string{"x@spbstu.ru"}}

	send["account"] = "petrov.pp"
	first, rest, isErr := call(t, c, "mail_send", send)
	if !isErr || !strings.Contains(rest, "refused") || first != "Аккаунт: "+own {
		t.Fatalf("a write as another account went through: %q %q %v", first, rest, isErr)
	}

	delete(send, "account")
	if _, rest, isErr := call(t, c, "mail_send", send); !isErr {
		t.Fatalf("a write without account went through: %q", rest)
	}

	send["account"] = "ivanov.ii"
	if first, rest, isErr := call(t, c, "mail_send", send); isErr || first != "Аккаунт: "+own || !strings.HasPrefix(rest, "Saved in Drafts") {
		t.Fatalf("a write as the active account failed: %q %q %v", first, rest, isErr)
	}
}

func TestSwitch(t *testing.T) {
	c := connect(t)
	if first, rest, isErr := call(t, c, "account_switch", map[string]any{"account": "petrov.pp"}); isErr || first != "Аккаунт: "+classmat {
		t.Fatalf("switch: %q %q", first, rest)
	}
	if first, _, _ := call(t, c, "mail_folders", nil); first != "Аккаунт: "+classmat {
		t.Fatalf("a read after the switch ran as %q", first)
	}
	if _, rest, isErr := call(t, c, "mail_mark", map[string]any{"account": own, "ids": []string{"1"}, "read": true}); !isErr {
		t.Fatalf("a write as the previous account went through: %q", rest)
	}
	if _, rest, isErr := call(t, c, "account_remove", map[string]any{"account": classmat}); !isErr {
		t.Fatalf("the active account was removed: %q", rest)
	}
	if _, rest, isErr := call(t, c, "account_switch", map[string]any{"account": "sidorov.ss"}); !isErr {
		t.Fatalf("switched to an account that is not added: %q", rest)
	}
	if _, rest, _ := call(t, c, "account_list", nil); !strings.Contains(rest, `"account": "petrov.pp@edu.spbstu.ru",
  "active": true`) {
		t.Fatalf("account_list: %s", rest)
	}
}
