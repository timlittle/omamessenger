# Security

## Reporting a vulnerability

Report a vulnerability through GitHub's private vulnerability reporting: open the [Security tab](https://github.com/timlittle/omamessenger/security) on this repository and click **Report a vulnerability**. This opens a private advisory that only maintainers can see; do not open a public issue for a security problem.

Include what you found, how to reproduce it, and the version of the helper and plugin you tested. Do not include message contents, phone numbers, QR codes or session files from a real account in the report.

## Scope

In scope:

- The Go helper (`backend/`), including the Telegram connector and its handling of sessions, credentials and message data
- The Omarchy plugin (`ui/`, `manifest.json`) and the install and release scripts (`scripts/`)

Out of scope:

- Telegram's own servers and infrastructure
- Vulnerabilities that require physical access to an already-unlocked machine

## Rewards

There is no bounty program. Reports are reviewed and fixed on a best-effort basis.
