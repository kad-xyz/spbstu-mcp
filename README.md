# spbstu-mcp

MCP server for the SPbPU student services:

- **dl.spbstu.ru** (Moodle): courses, course contents, assignments, deadlines, submission status, grades, forums, pages, file downloads.
- **my.spbstu.ru**: in progress.

## Credentials

The server signs in through the SPbPU single sign-on with a login stored in the macOS keychain as a generic password item:

```sh
security add-generic-password -s spbstu -a <login>@edu.spbstu.ru -T /usr/bin/security -w
```

`-w` must come last, so that `security` prompts for the password. Sessions (the Moodle mobile token, my.spbstu.ru cookies) are kept in `~/.config/spbstu-mcp/state.json` (mode 0600), so the server signs in again only when a session expires.

## Build and run

```sh
go build -o bin/spbstu-mcp ./cmd/spbstu-mcp
claude mcp add spbstu -s user -- "$PWD/bin/spbstu-mcp"
```

Flags: `-keychain-service` (default `spbstu`), `-downloads` (default `~/Downloads/spbstu`).
