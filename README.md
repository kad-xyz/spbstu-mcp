# spbstu-mcp

MCP server for the SPbPU student services. `study_overview` sums up the semester in one call: dl deadlines, openedu.ru progress and unread mail.

- **dl.spbstu.ru** (Moodle): courses, course contents, assignments, deadlines, submission status, grades, forums, pages, file downloads; submitting work to assignments and posting to forums with attachments.
- **mail.spbstu.ru** (Exchange Web Services): list and search mail, read messages, save attachments, write new messages and replies. `mail_send` only saves a draft unless `send` is true.
- **openedu.ru** (Открытое образование, Open edX): courses, progress and grades, due dates, course outline. Signs in through the SPbPU option of the openedu.ru SSO.
- **my.spbstu.ru**: in progress.

## Credentials

The server signs in through the SPbPU single sign-on with a login stored in the macOS keychain as a generic password item:

```sh
security add-generic-password -s spbstu -a <login>@edu.spbstu.ru -T /usr/bin/security -w
```

`-w` must come last, so that `security` prompts for the password. The SSO and my.spbstu.ru take the login without its domain; Exchange takes the full address. Sessions (the Moodle mobile token, openedu.ru and my.spbstu.ru cookies) are kept in `~/.config/spbstu-mcp/state.json` (mode 0600), so the server signs in again only when a session expires.

## Build and run

```sh
go build -o bin/spbstu-mcp ./cmd/spbstu-mcp
claude mcp add spbstu -s user -- "$PWD/bin/spbstu-mcp"
```

Flags: `-keychain-service` (default `spbstu`), `-downloads` (default `~/Downloads/spbstu`).
