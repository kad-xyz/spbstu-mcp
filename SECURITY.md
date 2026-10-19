# Security policy

spbstu-mcp handles university credentials and sessions, so security reports are taken seriously.

## Reporting a vulnerability

Please **do not open a public issue**. Report it privately through
[GitHub security advisories](https://github.com/kad-xyz/spbstu-mcp/security/advisories/new).
You should get an answer within a week.

Useful things to include: what an attacker could do, the steps to reproduce, and the version (`v0.x.y` or commit).

## Scope

In scope: leaks of the password, tokens or cookies (in tool results, logs, files or network traffic to anyone other
than the university and openedu.ru), weak permissions on `~/.config/spbstu-mcp`, write actions that run without the
documented confirmation.

Out of scope: vulnerabilities of the SPbPU or openedu.ru services themselves; report those to the university.
