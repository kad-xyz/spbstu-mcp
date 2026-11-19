package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kad-xyz/spbstu-mcp/internal/accounts"
	"github.com/kad-xyz/spbstu-mcp/internal/demo"
	"github.com/kad-xyz/spbstu-mcp/internal/keychain"
	"github.com/kad-xyz/spbstu-mcp/internal/moodle"
	"github.com/kad-xyz/spbstu-mcp/internal/openedu"
)

const oeEmail = "ivanov.ii@example.com"

// loginOpenEdu records the sign-ins openedu_login asks for and fails them with err.
type loginOpenEdu struct {
	demo.OpenEdu
	got *[]string
	err *error
}

func (o loginOpenEdu) SignInAs(_ context.Context, email string) (string, error) {
	*o.got = append(*o.got, email)
	return "ivanov_ii", *o.err
}

// loginFixture serves openedu_login and the account tools for two added accounts over a
// made-up credential store of openedu.ru passwords.
type loginFixture struct {
	c       *mcp.ClientSession
	reg     *accounts.Registry
	signIns []string
	err     error             // what the next sign-ins fail with
	stored  map[string]string // openedu.ru passwords by email
	asked   []string          // the sites the password dialog named
}

func newLoginFixture(t *testing.T) *loginFixture {
	t.Helper()
	f := &loginFixture{stored: map[string]string{}}
	old := oePasswords
	t.Cleanup(func() { oePasswords = old })
	oePasswords.load = func(service, email string) (keychain.Credentials, error) {
		if p, ok := f.stored[email]; ok && service == "spbstu-mcp-test-openedu" {
			return keychain.Credentials{Account: email, Password: p}, nil
		}
		return keychain.Credentials{}, errors.New("no such item")
	}
	oePasswords.ask = func(site, email string) (string, error) {
		f.asked = append(f.asked, site)
		return "typed", nil
	}
	oePasswords.save = func(_, email, password string) error { f.stored[email] = password; return nil }
	oePasswords.remove = func(_, email string) error { delete(f.stored, email); return nil }

	f.reg = &accounts.Registry{Dir: t.TempDir(), First: func() (string, error) { return own, nil }}
	if err := f.reg.Add(classmat); err != nil {
		t.Fatal(err)
	}
	a := newAccountSet(f.reg, "spbstu-mcp-test", false, nil)
	for _, acc := range []string{own, classmat} {
		a.sessions[acc] = &session{account: acc, sites: []string{moodle.DefaultSite}, dl: map[string]dlService{moodle.DefaultSite: demo.DL{}},
			oe: loginOpenEdu{got: &f.signIns, err: &f.err}, mail: demo.Mail{}, my: demo.My{}}
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	s.AddReceivingMiddleware(a.middleware)
	registerAccounts(s, a)
	registerOpenEduLogin(s, a, openeduProxy{})
	ct, st := mcp.NewInMemoryTransports()
	if _, err := s.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	c, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	f.c = c
	return f
}

func TestOpenEduLogin(t *testing.T) {
	f := newLoginFixture(t)
	first, rest, isErr := call(t, f.c, "openedu_login", map[string]any{"account": own})
	if isErr || first != "Аккаунт: "+own || !strings.Contains(rest, "«Политех»") || len(f.signIns) != 1 || f.signIns[0] != "" {
		t.Fatalf("back to «Политех»: %q %q %v, sign-ins %q", first, rest, isErr, f.signIns)
	}
	if _, rest, isErr := call(t, f.c, "openedu_login", map[string]any{"account": own, "email": "ivanov_ii"}); !isErr || len(f.signIns) != 1 {
		t.Fatalf("a login without @ went through: %q, sign-ins %q", rest, f.signIns)
	}
	_, rest, isErr = call(t, f.c, "openedu_login", map[string]any{"account": own, "email": oeEmail})
	if isErr || !strings.Contains(rest, oeEmail) || len(f.signIns) != 2 || f.signIns[1] != oeEmail {
		t.Fatalf("sign-in with email: %q %v, sign-ins %q", rest, isErr, f.signIns)
	}
	if f.stored[oeEmail] != "typed" || len(f.asked) != 1 || f.asked[0] != "openedu.ru" {
		t.Errorf("stored %q, the dialog named %q", f.stored, f.asked)
	}
}

// The tool changes which openedu.ru account an account writes to, so it takes account as
// the other writes do: a classmate's account is not tied to an openedu.ru account unnoticed.
func TestOpenEduLoginNeedsTheActiveAccount(t *testing.T) {
	f := newLoginFixture(t)
	for _, args := range []map[string]any{
		{"email": oeEmail},
		{"email": oeEmail, "account": classmat},
		{"account": classmat},
	} {
		if _, rest, isErr := call(t, f.c, "openedu_login", args); !isErr || len(f.signIns) != 0 || len(f.asked) != 0 {
			t.Errorf("%v went through: %q, sign-ins %q, dialogs %q", args, rest, f.signIns, f.asked)
		}
	}
}

// Only a password openedu.ru called wrong is removed, whether typed now or stored before.
func TestOpenEduLoginKeepsThePassword(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		stored bool // the password was stored before the call
		kept   bool
		says   string
	}{
		{"a network error with a password typed now", errors.New("dial tcp: i/o timeout"), false, true, "kept"},
		{"HTTP 502 with a stored password", errors.New("courses.openedu.ru/api/user/v1/me: HTTP 502"), true, true, "kept"},
		{"a block with a stored password", fmt.Errorf("%w: Account is temporarily disabled", openedu.ErrLocked), true, true, "wait"},
		{"a block with a password typed now", fmt.Errorf("%w: Account is temporarily disabled", openedu.ErrLocked), false, true, "wait"},
		{"a wrong password typed now", fmt.Errorf("%w: Invalid username or password.", openedu.ErrRejected), false, false, "removed"},
		{"a wrong stored password", openedu.ErrRejected, true, false, "removed"},
	} {
		f := newLoginFixture(t)
		if tc.stored {
			f.stored[oeEmail] = "stored"
		}
		f.err = tc.err
		_, rest, isErr := call(t, f.c, "openedu_login", map[string]any{"account": own, "email": oeEmail})
		if _, kept := f.stored[oeEmail]; !isErr || kept != tc.kept || !strings.Contains(rest, tc.says) || len(f.asked) != map[bool]int{true: 0, false: 1}[tc.stored] {
			t.Errorf("%s: kept %v, want %v; dialogs %d; reply %q", tc.name, kept, tc.kept, len(f.asked), rest)
		}
	}
}

// An openedu.ru password belongs to the account that signs in with it: it goes when the
// account signs in another way and when the account is removed.
func TestOpenEduPasswordHasAnOwner(t *testing.T) {
	f := newLoginFixture(t)
	const other = "petrov.pp@example.com"
	f.stored[oeEmail] = "stored"
	if err := f.reg.Store(own).Set("openedu.login", oeEmail); err != nil {
		t.Fatal(err)
	}
	if _, rest, isErr := call(t, f.c, "openedu_login", map[string]any{"account": own, "email": other}); isErr {
		t.Fatal(rest)
	}
	if _, kept := f.stored[oeEmail]; kept || f.stored[other] != "typed" {
		t.Errorf("after a switch to another email: %q", f.stored)
	}
	if err := f.reg.Store(own).Set("openedu.login", other); err != nil {
		t.Fatal(err)
	}
	if _, rest, isErr := call(t, f.c, "openedu_login", map[string]any{"account": own}); isErr {
		t.Fatal(rest)
	}
	if len(f.stored) != 0 {
		t.Errorf("after going back to «Политех»: %q", f.stored)
	}

	// A failed switch leaves the password in use where it is.
	f.stored[oeEmail] = "stored"
	if err := f.reg.Store(own).Set("openedu.login", oeEmail); err != nil {
		t.Fatal(err)
	}
	f.err = errors.New("dial tcp: i/o timeout")
	if _, rest, isErr := call(t, f.c, "openedu_login", map[string]any{"account": own}); !isErr {
		t.Fatal(rest)
	}
	if f.stored[oeEmail] != "stored" {
		t.Errorf("after a failed switch: %q", f.stored)
	}

	f.stored["sidorov@example.com"] = "stored"
	if err := f.reg.Store(classmat).Set("openedu.login", "sidorov@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, rest, isErr := call(t, f.c, "account_remove", map[string]any{"account": classmat}); isErr {
		t.Fatal(rest)
	}
	if _, kept := f.stored["sidorov@example.com"]; kept || f.stored[oeEmail] != "stored" {
		t.Errorf("after account_remove: %q", f.stored)
	}
}

func TestOpenEduLoginDemo(t *testing.T) {
	a := newAccountSet(nil, "spbstu-mcp-test", true, nil)
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
	if _, rest, isErr := call(t, c, "openedu_login", map[string]any{"account": demoAccount, "email": oeEmail}); !isErr || rest != errDemo.Error() {
		t.Fatalf("demo mode: %q %v", rest, isErr)
	}
}
