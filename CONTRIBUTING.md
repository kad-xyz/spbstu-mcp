# Contributing to spbstu-mcp

Thanks for helping! Bug reports, fixes and support for more SPbPU services are all welcome.
Issues and pull requests can be written in English or Russian.

## Ground rules

- **Never commit personal data.** No logins, passwords, tokens, cookies, student or group numbers, real names from
  mail or courses, and no `state.json`. Use made-up data in tests (`ivanov.ii@edu.spbstu.ru`).
- **Be gentle with university services.** Sign in once and reuse the session: the sites lock accounts after
  frequent sign-ins. Never sign in inside a loop or on every tool call.
- **Writes need confirmation.** A tool that changes anything on a university service must say so in its
  description (it is shown to the model) and carry `DestructiveHint`.
- **No exam automation.** Tools that take graded tests or proctored exams will not be accepted.

## Getting started

```sh
git clone https://github.com/kad-xyz/spbstu-mcp && cd spbstu-mcp
go test ./...
go build -o bin/spbstu-mcp ./cmd/spbstu-mcp
claude mcp add spbstu-dev -- "$PWD/bin/spbstu-mcp"
```

Live tests run against the real services with your own keychain item (see the README):

```sh
SPBSTU_LIVE=1 go test ./internal/moodle ./internal/openedu
```

## Code

- Go, standard library first; the only dependency is the official MCP Go SDK. Ask in an issue before adding another.
- `gofmt`, `go vet` and `go test ./...` must pass; CI checks all three on macOS.
- One package per service under `internal/`, MCP tool definitions in `cmd/spbstu-mcp`.
- Tool results are compact JSON for a model to read: drop fields nobody needs, turn HTML into text, dates into
  local `YYYY-MM-DD HH:MM`.
- Comments explain why, not what.

### Adding a service

1. Find how the site signs in. Most SPbPU services go through `cas.spbstu.ru`; reuse `internal/cas`.
2. Keep the session in a `state.Jar` (or the `state.Store`) so that it survives restarts.
3. Prefer a JSON API (Moodle web services, Open edX) over scraping HTML.
4. Add offline tests for parsing with recorded, anonymised responses, and a live test behind `SPBSTU_LIVE`.

## Pull requests

- Branch from `main`, keep a PR to one change, and describe what changed and why.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org): `feat:`, `fix:`, `docs:`, `chore:`.
- Mention how you tested it: the tests you added, and the tool calls you made against the live service.

## Releases

Maintainers tag `vX.Y.Z` on `main`; GitHub Actions builds the macOS binaries and publishes the release.
