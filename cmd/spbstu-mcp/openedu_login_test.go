package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/accounts"
	"github.com/kad-xyz/spbstu-mcp/internal/demo"
)

// loginOpenEdu records the sign-ins openedu_login asks for.
type loginOpenEdu struct {
	demo.OpenEdu
	got *[]string
}

func (o loginOpenEdu) SignInAs(_ context.Context, email string) (string, error) {
	*o.got = append(*o.got, email)
	return "ivanov_ii", nil
}

func TestOpenEduLogin(t *testing.T) {
	reg := &accounts.Registry{Dir: t.TempDir(), First: func() (string, error) { return own, nil }}
	a := newAccountSet(reg, "spbstu-mcp-test", false)
	var got []string
	a.sessions[own] = &session{account: own, dl: demo.DL{}, oe: loginOpenEdu{got: &got}, mail: demo.Mail{}, my: demo.My{}}
	s := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	s.AddReceivingMiddleware(a.middleware)
	registerOpenEduLogin(s, a, openeduProxy{})
	ct, st := mcp.NewInMemoryTransports()
	if _, err := s.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	c, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	first, rest, isErr := call(t, c, "openedu_login", nil)
	if isErr || first != "Аккаунт: "+own || !strings.Contains(rest, "«Политех»") || len(got) != 1 || got[0] != "" {
		t.Fatalf("back to «Политех»: %q %q %v, sign-ins %q", first, rest, isErr, got)
	}
	if _, rest, isErr := call(t, c, "openedu_login", map[string]any{"email": "ivanov_ii"}); !isErr || len(got) != 1 {
		t.Fatalf("a login without @ went through: %q, sign-ins %q", rest, got)
	}
}

func TestOpenEduLoginDemo(t *testing.T) {
	a := newAccountSet(nil, "spbstu-mcp-test", true)
	s := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	s.AddReceivingMiddleware(a.middleware)
	registerOpenEduLogin(s, a, openeduProxy{})
	ct, st := mcp.NewInMemoryTransports()
	if _, err := s.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	c, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, rest, isErr := call(t, c, "openedu_login", map[string]any{"email": "ivanov.ii@example.com"}); !isErr || !strings.Contains(rest, "-demo") {
		t.Fatalf("demo mode: %q %v", rest, isErr)
	}
}
