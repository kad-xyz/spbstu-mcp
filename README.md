<div align="center">

# spbstu-mcp

**Your Peter the Great St. Petersburg Polytechnic University (SPbPU) studies, available to Claude.**

An [MCP](https://modelcontextprotocol.io) server for SPbPU student services: the dl.spbstu.ru Moodle,
the university mailbox, openedu.ru and the timetable. You sign in with your university account; your password stays in your
operating system's credential store.

[![CI](https://github.com/kad-xyz/spbstu-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/kad-xyz/spbstu-mcp/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/kad-xyz/spbstu-mcp?sort=semver)](https://github.com/kad-xyz/spbstu-mcp/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/kad-xyz/spbstu-mcp)](go.mod)
![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**English** · [Русский](README.ru.md)

</div>

https://github.com/user-attachments/assets/79137c69-5d3f-4270-9164-e509f88c3535

---

> *"What's due this week?"* · *"Summarise the database course assignment and download its files."* ·
> *"Did the teacher grade my coursework?"* · *"Draft a reply to the curator's email."*

## Contents

- [What it does](#what-it-does)
- [Quick start](#quick-start)
- [Several accounts](#several-accounts)
- [Tools](#tools)
- [How it works](#how-it-works)
- [Security and privacy](#security-and-privacy)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [Contributing](#contributing)

## What it does

| | Service | Read | Write |
|---|---|---|---|
| 🎓 | **dl.spbstu.ru**, the Moodle LMS | courses, course contents, assignments with full task text, deadlines, submission status and feedback, grades, forums, pages, files | submit work to an assignment, post to forums with attachments |
| ✉️ | **mail.spbstu.ru**, Exchange | list and search mail, read messages, save attachments | write new messages and replies (saved as drafts by default), delete |
| 📚 | **openedu.ru**, Open edX | enrolled courses, progress and grades, due dates, course outline, page text and attached files | |
| 🗓 | **ruz.spbstu.ru**, the public timetable | weekly timetable of any group or teacher | |
| 🧭 | **Everything at once** | `study_overview`: dl deadlines, openedu.ru progress and unread mail in one call | |
| 🏛 | **my.spbstu.ru**, «Мой Политех» | record book grades with semester averages, study plan, this year's work plan | |
| 🔎 | **polyvsp.ru, polyphis.ru**, student answer bases | answers to test questions, with the source page | |

## Quick start

**1. Store your university login** in the operating system's credential store. `LOGIN` is your single university
account, the one in `LOGIN@edu.spbstu.ru`.

<details open>
<summary><b>macOS</b>: keychain</summary>

```sh
security add-generic-password -s spbstu -a LOGIN@edu.spbstu.ru -T /usr/bin/security -w
```

> [!IMPORTANT]
> Keep `-w` **last**, so that `security` prompts for the password with hidden input; anything after `-w` would be
> stored as the password itself. To check (prints only the length): `security find-generic-password -s spbstu -w | wc -c`

</details>

<details>
<summary><b>Linux</b>: Secret Service (GNOME Keyring, KWallet)</summary>

Needs `secret-tool` (`apt install libsecret-tools`, `dnf install libsecret`, `pacman -S libsecret`):

```sh
secret-tool store --label=spbstu service spbstu account LOGIN@edu.spbstu.ru
```

</details>

<details>
<summary><b>Windows</b>: Credential Manager</summary>

```powershell
cmdkey /generic:spbstu /user:LOGIN@edu.spbstu.ru /pass
```

</details>

The password is asked with hidden input in every case.

**2. Install the server.** With [Homebrew](https://brew.sh) (macOS, Linux):

```sh
brew install kad-xyz/tap/spbstu-mcp
```

Or download the archive for your system from [Releases](https://github.com/kad-xyz/spbstu-mcp/releases/latest)
and put `spbstu-mcp` somewhere on disk, e.g. `~/.local/bin`. On macOS, clear the download quarantine (the binary is not
notarized):

```sh
xattr -d com.apple.quarantine ~/.local/bin/spbstu-mcp
```

<details>
<summary>Or build from source (Go 1.26+)</summary>

```sh
go install github.com/kad-xyz/spbstu-mcp/cmd/spbstu-mcp@latest
```

</details>

**3. Connect it to Claude.**

<table>
<tr><th>Claude Code</th><th>Claude Desktop</th></tr>
<tr><td>

```sh
claude mcp add spbstu -s user -- "$(command -v spbstu-mcp)"
```

</td><td>

`claude_desktop_config.json` (macOS: `~/Library/Application Support/Claude/`, Windows: `%APPDATA%\Claude\`):

```json
{
  "mcpServers": {
    "spbstu": { "command": "/Users/YOU/.local/bin/spbstu-mcp" }
  }
}
```

</td></tr>
</table>

Then just ask: *"What's going on with my studies?"*

> [!TIP]
> **No account at hand?** Start the server with `-demo` (`claude mcp add spbstu-demo -- spbstu-mcp -demo`): it serves a
> made-up student's courses, deadlines and mail, touches no university service and needs no credentials.

## Several accounts

The server can keep more than one university account, say yours and a classmate's you are helping, and acts as one of
them at a time, the **active** one. Ask *"add the account ivanov.ii"*: on macOS a system dialog asks for its password
(paste it from the Passwords app), which goes straight to the keychain, never through the chat, and one sign-in checks
it. Then *"switch to ivanov.ii"*, and back when you are done.

- Every reply starts with `Аккаунт: LOGIN@edu.spbstu.ru`, the account it was made as.
- Every account has its own sessions, so one person's sign-in never serves another.
- Tools marked ✍️ take an `account` argument and refuse to run unless it is the active account: a mail cannot go out
  from a classmate's mailbox because the switch back was forgotten.
- The active account is remembered after a restart.

On Linux and Windows `account_add` returns the `secret-tool` or `cmdkey` command that stores the password; run it, then
call `account_add` again. The accounts live under the same credential store service, one item per account (on Windows,
an extra account is the generic credential `spbstu:LOGIN@edu.spbstu.ru`).

## Tools

<details open>
<summary><b>Accounts</b>: 4 tools</summary>

| Tool | Description |
|---|---|
| `account_list` | Added accounts and which one is active. |
| `account_switch` | Make another added account the active one. |
| `account_add` | Add an account: its password is typed into a system dialog (macOS), then checked with one sign-in. |
| `account_remove` | Remove an account that is not active: its password and saved sessions. |

</details>

<details>
<summary><b>Across services</b>: 3 tools</summary>

| Tool | Description |
|---|---|
| `study_overview` | dl.spbstu.ru deadlines (with recently overdue ones), progress in current openedu.ru courses and unread mail. A source that fails is reported, the rest still come back. |
| `study_subject` | Everything about one discipline: work-plan hours and control type, the dl course with assignments, quizzes, deadlines and teachers, grades received. |
| `study_export_ics` | Every upcoming dl and openedu due date as an `.ics` file with day-before reminders, for Apple, Google or Outlook Calendar. |

</details>

<details>
<summary><b>dl.spbstu.ru</b>: 20 tools</summary>

| Tool | Description |
|---|---|
| `dl_courses` | Courses you are enrolled in (current ones, or all). |
| `dl_course_contents` | Sections of a course with their activities, descriptions and file links. |
| `dl_deadlines` | Upcoming and recently overdue due dates across all courses. |
| `dl_assignments` | Assignments of a course: full task text, due dates, attached files. |
| `dl_submission_status` | Your submission for an assignment: status, files, grade, teacher feedback. |
| `dl_grades` | Your grades in a course. |
| `dl_forum_discussions` | Discussions of a forum. |
| `dl_discussion_posts` | All posts of a discussion, with attachments. |
| `dl_page` | Text of a page resource (exam rules, ticket lists). |
| `dl_download` | Download a file to `~/Downloads/spbstu`. |
| `dl_quizzes` | Quizzes of a course: opening and closing time, time limit, attempts allowed and used, best grade (information only). |
| `dl_teachers` | Teachers of a course and their last visit. |
| `dl_notifications` | Newest notifications: grades, feedback, forum posts, due dates. |
| `dl_messages` | Private conversations with their last message. |
| `dl_conversation` | All messages of a conversation. |
| `dl_search` | Search every course for activities, resources and files. |
| `dl_send_message` ✍️ | Send a private message, e.g. to a teacher. |
| `dl_submit_assignment` ✍️ | Upload files and/or text to an assignment, optionally submit for grading. |
| `dl_forum_new_discussion` ✍️ | Start a discussion, e.g. post a report to an upload forum. |
| `dl_forum_reply` ✍️ | Reply to a post. |

</details>

<details>
<summary><b>Mail</b>: 9 tools</summary>

| Tool | Description |
|---|---|
| `mail_list` | Newest messages of a folder; unread only; search (`from:name`, `subject:word`, plain words). |
| `mail_read` | A whole message as plain text, with recipients and attachments. |
| `mail_download_attachment` | Save an attachment to `~/Downloads/spbstu/mail`. |
| `mail_send` ✍️ | New message or reply, with file attachments (up to 20 MB). Saves a draft unless `send` is true. |
| `mail_delete` ✍️ | Move a message or draft to Deleted Items (restorable). |
| `mail_folders` | Mail folders with total and unread counts. |
| `mail_move` ✍️ | Move messages to another folder. |
| `mail_mark` ✍️ | Mark messages read or unread. |
| `mail_calendar` | Events of the mailbox calendar: meetings, webinars, consultations. |

</details>

<details>
<summary><b>my.spbstu.ru</b>: 5 tools</summary>

| Tool | Description |
|---|---|
| `my_grades` | Record book (зачётка): every grade with control type, date and lecturer; average of each semester. |
| `my_study_plan` | The programme's study plan: disciplines, semesters, hours, control type. |
| `my_work_plan` | This year's work plan by semester: lecture, practice, lab and self-study hours. |
| `my_record_books` | Record books, one per programme, with the group. |
| `my_debts` | Academic debts: disciplines whose latest grade is failing, and past ones with no grade to check. |

</details>

<details>
<summary><b>Timetable</b>: 1 tool</summary>

| Tool | Description |
|---|---|
| `schedule` | Week of a group or a teacher: time, type, teachers, room, webinar and dl links. Ambiguous names return the candidates. |

</details>

<details>
<summary><b>openedu.ru</b>: 14 tools</summary>

| Tool | Description |
|---|---|
| `openedu_courses` | Your enrolled courses. |
| `openedu_progress` | Completed units, current grade, grading policy, score of every graded subsection (`all` lists videos and notes too). |
| `openedu_dates` | Course start, end and due dates. |
| `openedu_outline` | Chapters and subsections with completion and links. |
| `openedu_unit` | Text and links of course pages: a page or a whole subsection (lab descriptions, notes, attached files). Takes a link, a block id or part of a name. |
| `openedu_problems` | Questions of a test or exercise page: question text (tables and pictures included), input names, choices, saved answers and their status, score and attempts. |
| `openedu_answer_many` ✍️ | Answers several questions in one call, as `openedu_answer` does each, and returns a short result per question (score, attempts, status). Confirm with the user first. |
| `openedu_answer` ✍️ | Saves the user's answers to a question, or with `submit` checks them for a score (spends one attempt; a wrongly answered question is reset first). Confirm with the user first. |
| `openedu_exam` ✍️ | A timed exam: time limit, due date, the state of your attempt; while it runs, the minutes left and its questions, answered with `openedu_answer`. With `start`, starts the exam, whose clock cannot be stopped. Confirm with the user first. |
| `openedu_search` | Finds course runs in the openedu.ru catalog by words of the name: run id, dates, end of enrollment. |
| `openedu_enroll` ✍️ | Enrolls in a course run by its id, e.g. `course-v1:spbstu+WEBPYT+fall_2026`, as the «Записаться» button does. Confirm with the user first. |
| `openedu_forum` | A course forum thread: its text and every response. Takes the thread link. |
| `openedu_forum_reply` ✍️ | Posts a response to a course forum thread. Confirm with the user first. |
| `openedu_exam_finish` ✍️ | Finishes the running attempt of a timed exam. Confirm with the user first. |

</details>

✍️ *changes something on a university service; see [Security and privacy](#security-and-privacy).*

<details>
<summary><b>Answer bases</b>: 2 tools</summary>

| Tool | Description |
|---|---|
| `answers_subjects` | Courses of polyvsp.ru and polyphis.ru, filtered by part of the name. |
| `answers_search` | Looks questions up on the chosen course pages: up to three close entries per question with the answer, source link and a 0..1 score; `found` when it is the same question. |

With a link to an openedu.ru test, the model looks every question up, saves the answers without submitting them and reports each question as ✅ from the base, 🟡 from the base but doubtful (the base answer is kept anyway) or ⚠️ not in the base, to be checked.

</details>

## How it works

```mermaid
flowchart LR
    C[Claude] -- MCP / stdio --> S[spbstu-mcp]
    K[(OS credential store<br/>item "spbstu")] -. login + password .-> S
    S -- SSO --> CAS[cas.spbstu.ru]
    CAS --> DL[dl.spbstu.ru<br/>Moodle mobile API]
    S -- SSO --> KC[sso.openedu.ru<br/>«Политех» provider] --> CAS
    KC --> OE[courses.openedu.ru<br/>Open edX API]
    S -- EWS, Basic auth over TLS --> M[mail.spbstu.ru<br/>Exchange]
    S <-. sessions .-> ST[(~/.config/spbstu-mcp/accounts<br/>one state.json per account, 0600)]
```

- **dl.spbstu.ru**: signs in through the university SSO and obtains a Moodle mobile web service token, the same way the official Moodle app does.
- **openedu.ru**: signs in through the «Политех» identity provider of the openedu.ru SSO, then reads the Open edX course APIs.
- **Mail**: Exchange Web Services with the same account.
- **my.spbstu.ru**: its own sign-in form, then the JSON endpoints its pages use.
- **Answer bases**: public pages, cached for a week in `~/.config/spbstu-mcp/answers`; when polyvsp.ru is down its web.archive.org copy is read.
- **Sessions** (the Moodle token and cookies) are kept between runs, so the server signs in again only when a session has expired. University sites lock accounts after frequent sign-ins.

## Security and privacy

- 🔑 **The password never leaves your Mac except to SPbPU and openedu.ru sign-in pages.** It is read from the credential store at sign-in time and never appears in tool results.
- 🗂 **Sessions** are stored in `~/.config/spbstu-mcp/accounts/LOGIN@edu.spbstu.ru/state.json`, one file per account, readable only by you. Delete a file to sign that account out everywhere.
- 👥 **Accounts don't mix.** Every write tool refuses to run unless its `account` argument is the active account; see [Several accounts](#several-accounts).
- ✋ **Writes need your confirmation.** The descriptions of `dl_submit_assignment`, `dl_forum_*` and `mail_send` tell the model to act only after you have confirmed exactly what goes where. `mail_send` only saves a draft unless asked to send.
- 🚫 **No exams.** Timed and proctored exams open only in the browser. Test answers are only saved; submitting them is up to you.

> [!WARNING]
> This is an unofficial project, not affiliated with SPbPU or openedu.ru. Use it with your own account only.

## Configuration

| Flag | Default | Description |
|---|---|---|
| `-keychain-service` | `spbstu` | Name of the credential store service with the university logins, one item per account. |
| `-downloads` | `~/Downloads/spbstu` | Where downloaded files and attachments are saved. |
| `-group` | | Default group for `schedule`, e.g. `5130902/30201`. |
| `-demo` | `false` | Serve made-up data instead of the university services; no credentials needed. |

## Troubleshooting

| Symptom | Fix |
|---|---|
| `keychain item "spbstu" not found` (or `Secret Service item`, `credential`) | Create it, see [Quick start](#quick-start). |
| `CAS rejected the credentials` | Check the stored password length; re-create the item with `-w` last. |
| `mail.spbstu.ru rejected the credentials` | The keychain account must be the full address, `LOGIN@edu.spbstu.ru`. |
| `refused: account is …, but the active account is …` | A write was asked as another account. Switch with `account_switch` if that is what you meant. |
| Sign-in keeps failing after many attempts | The site may have locked sign-ins for a while. Wait, then try once. |
| macOS: *"cannot be opened because the developer cannot be verified"* | `xattr -d com.apple.quarantine ~/.local/bin/spbstu-mcp` |

## Development

```sh
go test ./...                    # offline unit tests
SPBSTU_LIVE=1 go test ./...      # plus checks against the live services (needs the credential item)
go build -o bin/spbstu-mcp ./cmd/spbstu-mcp
```

```
cmd/spbstu-mcp     MCP tools (dl, mail, openedu, overview, accounts)
internal/accounts  added accounts, the active one, per-account session stores
internal/cas       university SSO sign-in
internal/moodle    dl.spbstu.ru client
internal/ews       Exchange Web Services client
internal/openedu   openedu.ru and Open edX client
internal/mypoly    my.spbstu.ru client
internal/ruz       public timetable client
internal/answers   polyvsp.ru and polyphis.ru answer bases
internal/keychain  credential stores: macOS keychain, Secret Service, Windows Credential Manager
internal/state     persisted sessions and cookie jar
```

Releases are built by GitHub Actions on every `v*` tag.

## Contributing

Contributions are welcome, in English or Russian: read [CONTRIBUTING.md](CONTRIBUTING.md) first.
Security issues go through [private advisories](SECURITY.md), not public issues.

## License

[MIT](LICENSE)
