# Contributing to OmaMessenger

OmaMessenger is an Omarchy plugin first and a messaging client second. Keep it native to the running Omarchy shell, let Hyprland manage its window, and keep messaging protocols behind the Go connector boundary.

## Setup

Needs Go 1.26+, Node 22+, and, for QML lint and the offscreen tests, Omarchy's shell and Qt 6.

```sh
make help            # list the commands
make check           # every gate: build, tests with coverage, lint
make build           # build the helper into bin/dev/, which the launcher prefers
make install-local   # install this checkout into Omarchy and enable it
```

## Before changing code

- Read [AGENTS.md](AGENTS.md) for the project rules, [.claude/rules/](.claude/rules/) for the coding standards and [FORGE_SPEC.md](FORGE_SPEC.md) for product scope.
- The UI uses only the helper's API (see [docs/api.md](docs/api.md)). It never speaks WhatsApp or Telegram protocol details.
- A service-specific capability belongs in its connector. Do not show a control for something a service cannot do.
- Keep credentials and message content out of logs, configuration and test output. Tests use temporary directories and fake data.

## UI and interaction

The window has three areas: a service rail, the conversation list and the open conversation. Keep them usable at the minimum window size, and make every action work from both keyboard and mouse. Use Omarchy's theme tokens and `qs.Ui` controls. When a visible interaction changes, update [docs/shortcuts.md](docs/shortcuts.md) (and the README's essentials table, if it changed) and check the affected states.

## Tests

Write the failing test first; a bug fix starts with a test that reproduces it. Put each test at the narrowest layer that shows the behaviour:

| Change | Where the test goes |
| --- | --- |
| Pure UI logic in `ui/lib` | `tests/unit/<file>.test.cjs`, run with node |
| Storage | `backend/internal/store/<file>_test.go`, on a real SQLite database in a temporary directory |
| What the client does | `backend/internal/app/<file>_test.go`, with a real store and fake connectors |
| Notification decisions | `backend/internal/app/policy`, every combination |
| Connector supervision and the fake connectors | `backend/internal/connector/...`, inside `testing/synctest` |
| The helper API | `backend/internal/server`, through a real JSON-RPC client; `backend/main_test.go` drives the built binary |
| Allowed imports, size and complexity | `.golangci.yml` |
| No message content in logs | `tools/nologcontent` |
| Helper install and launcher | `tests/unit/helper-install.test.cjs`, against a fake release |

Before opening a pull request, run:

```sh
make check
```

It builds the helper, runs the Go tests with the race detector and the coverage gates in `.testcoverage.yml`, runs the JavaScript tests, and lints Go, shell scripts and QML. It does not start a compositor or check the rendered UI.

## Checking visual changes

Automated checks cover logic and QML validity, not what the window looks like. Run `make install-local` on an Omarchy machine, open the plugin, and try the changed flow with keyboard and mouse. Include the theme, window size and a screenshot in the pull request when you can. Never claim a check you did not do.

## Pull requests

Keep each pull request to one user-visible change or one maintenance task. Include:

- the problem, with a short reproduction or the reason for the design
- what changed, and any service-specific limits
- the verification commands you ran and their results
- a screenshot for visual changes

If an AI tool helped, you remain responsible for reviewing and understanding the whole diff.
