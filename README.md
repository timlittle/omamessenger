# OmaMessenger

OmaMessenger is an Omarchy-native, keyboard-first messaging client. Omarchy summons its normal, non-modal Quickshell window; the UI and Go helper communicate over local JSON-lines IPC on the helper's standard input and output. The helper owns persistence and the protocol adapter boundary; the client uses normalized accounts, conversations, and messages.

## Current status

The Go helper is complete for an offline seeded demo (`--demo`): local sends, scripted delivery states, failures and retries, replies, typing indicators, notifications and unread counts. It is covered by race-tested Go tests and architecture checks.

**The plugin UI is not usable yet.** The window in the repository root (`Panel.qml`, `Service.qml`) is the original scaffold. It still expects an HTTP API on port 43821, and its service starts the helper without a stdin pipe, so the new stdio helper exits at once. Installing the plugin today opens a window that cannot load conversations. Phase B in [docs/TASKS.md](docs/TASKS.md) replaces this UI.

Real WhatsApp and Telegram authentication, remote sync and delivery are not connected yet, so this project does not replace those desktop clients.

The planned Go clients are `whatsmeow` for WhatsApp and `gotd/td` for Telegram. Media and reactions follow a reliable text workflow.

## Requirements

- Omarchy with `omarchy-shell` and the third-party plugin manager
- Linux x86_64 or ARM64

## Install on Omarchy

Installation works mechanically, but see Current status: the UI does not work with the current helper until Phase B. Install the plugin with Omarchy:

```sh
omarchy plugin add https://github.com/timlittle/omamessenger --enable
```

Open or focus OmaMessenger:

```sh
omarchy-shell shell summon io.github.omamessenger '{}'
```

The bundled helper starts automatically when the plugin loads in `omarchy-shell`; no Go compiler or separate service setup is needed. The binaries currently target Linux x86_64 and ARM64. There is no separate installer or systemd unit.

Remove the plugin with:

```sh
omarchy plugin remove io.github.omamessenger
```

## Accounts and authentication

Remote authentication is not implemented. To run the seeded demo helper from a checkout:

```sh
go run -mod=vendor ./backend --demo --no-chatter --seed 1 --data-dir "$(mktemp -d)"
```

The helper reads one JSON request per line from stdin and writes responses and events to stdout. `--demo` uses `demo.db`; normal mode uses `messages.db`. Both live under `${XDG_DATA_HOME:-$HOME/.local/share}/omamessenger/` unless `--data-dir` or `--db` is supplied. Data is private to the current user. No bearer token, TCP listener, or system service is used.

An earlier scaffold used `~/.config/omamessenger/messages.db` and `api.token`; those files are not migrated. You may remove them if you no longer need that scaffold data.

The demo seeds three local accounts and eleven conversations each time it starts; stable remote IDs prevent duplicate messages. `--seed N` makes scripted chatter deterministic, while `--no-chatter` disables unsolicited demo messages. This mode never logs message contents or credentials.

## Keyboard shortcuts

These describe the scaffold UI that Phase B replaces; the new keymap is specified in `docs/TASKS.md` (C6).

- `j` / `k`: move through conversations
- `Enter`: open the selected conversation
- `Esc`: return to the list, then close the panel
- `Ctrl+K`: focus conversation search
- `Ctrl+N`: create a local conversation
- `Ctrl+1` / `Ctrl+2` / `Ctrl+0`: WhatsApp / Telegram / all conversations
- Mouse clicks work for filters, conversation rows, and message controls

The compose dialog expects an account ID and contact/chat label. Without service connectors, it creates only a local conversation.

## Development

Build or run the Go helper directly:

```sh
go build -mod=vendor -o /tmp/oma-messenger-service ./backend
go run -mod=vendor ./backend --demo --no-chatter --seed 1
```

The transport is stdio JSON lines (protocol version 1); the method and event reference is generated into [docs/PROTOCOL.md](docs/PROTOCOL.md). The helper exits cleanly when stdin closes or on SIGTERM. Service connectors publish normalized messages and account state through the connector interface; real connectors are planned but are not implemented yet.

The root Makefile provides the regular development workflow:

```sh
make help             # list all targets
make build            # build the helper for this machine into bin/dev/
make test             # build, Go tests (race + coverage gates), JS tests, lint, docs checks
make docs-check       # check docs against the code
make status           # inspect the current worktree
make pull             # fast-forward from the configured upstream
make install-local    # copy this checkout into Omarchy and enable it
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for project boundaries, the test map, UI review guidance, and pull request expectations. `AGENTS.md` contains the working rules used by both human and AI contributors.

`make test` requires Go 1.26+, Node 22+, and QML lint tools. Go tests run with the race detector against temporary databases; `make coverage` enforces per-package statement gates (`tools/covergate`, excluding only the helper's `func main`). `make lint` runs gofmt, go vet and the architecture rules in `tools/omalint`; `backend/internal/archtest` checks coupling and cohesion as part of the Go tests. `make test` does not launch a compositor or verify rendered pixels.

`make install-local` builds the native helper, stages the plugin runtime files into `~/.config/omarchy/plugins/io.github.omamessenger/`, validates it, asks the running Omarchy shell to rescan, and enables the plugin. Then open it with `omarchy-shell shell summon io.github.omamessenger '{}'`. This installs your current working tree so you can try changes before pushing.

Build both bundled Linux helper binaries from source with Go 1.26 or newer:

```sh
./scripts/build-release.sh
```

GitHub Actions rebuilds and commits the bundled binaries when backend source changes on `main`. Pushing a `v*` tag also creates a GitHub Release with both architecture builds attached. The repository’s Actions workflow permissions must allow read and write access to contents for the binary commit and release steps. Plugin installs use the binaries tracked in the repository, so no release download or first-run setup is needed.

## Architecture

- `backend/`: Go stdio JSON-lines helper, normalized domain, SQLite persistence, and connector boundary
- `tools/`: rules enforced in code (Go analyzers, coverage gates, docs checks)
- `docs/`: the build plan, the generated protocol reference and the generated architecture metrics
- `Panel.qml`, `Service.qml`: the scaffold Quickshell panel and helper lifecycle, replaced in Phase B
- `bin/`: bundled Linux x86_64 and ARM64 helpers plus architecture selector
- `scripts/build-release.sh`: offline cross-build for bundled helpers
- `Makefile`: build, test, coverage, validation, local install, status, and pull targets
- `manifest.json`: Omarchy plugin manifest
- `vendor/`: pinned Go dependencies for offline builds

The panel and service entry points run inside the existing `omarchy-shell` process. The Go helper is a child process supervised by the shell and communicates with the UI through local JSON-lines IPC. Theme colors and styles come from Omarchy's live tokens.
