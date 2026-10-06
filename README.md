# OmaMessenger

OmaMessenger is a keyboard-first messaging client for [Omarchy](https://omarchy.org), for WhatsApp and Telegram. It runs as an Omarchy plugin: the window is a normal Quickshell window inside `omarchy-shell`, and a small Go helper keeps the message database and talks to the messaging services.

## Status

- **Helper:** complete for an offline demo (`--demo`): seeded accounts and conversations, sending with delivery receipts, failures and retries, replies, typing indicators, notifications and unread counts.
- **UI:** in progress in `ui/`. The service rail, conversation list, conversation view and bar icon exist as components and the panel runs the helper end to end, but the full keyboard-routed layout from [docs/plan.md](docs/plan.md) is not wired in yet.
- **WhatsApp and Telegram:** not connected yet. The planned libraries are [whatsmeow](https://github.com/tulir/whatsmeow) and [gotd/td](https://github.com/gotd/td).

## Requirements

- Omarchy with `omarchy-shell` and its plugin manager
- Linux x86_64 or ARM64

## Install

```sh
omarchy plugin add https://github.com/timlittle/omamessenger --enable
~/.config/omarchy/plugins/io.github.omamessenger/scripts/install-helper.sh
```

The second command installs the helper:

- It downloads the release named in `helper-version` from GitHub Releases, checks it against the release's `SHA256SUMS`, and confirms it reports that version.
- It installs it into `~/.local/share/omamessenger/bin/`; you do not need Go.
- Nothing is downloaded until you run it. Running it again does nothing, and `--status` reports whether the helper is installed.
- **No release has been published yet**, so until `v0.3.0` is tagged, build the helper from a checkout with `make build`.

Open the window with `omarchy-shell shell summon io.github.omamessenger '{}'`. Remove the plugin with `omarchy plugin remove io.github.omamessenger`.

## Data and privacy

The helper keeps its database in `${XDG_DATA_HOME:-~/.local/share}/omamessenger/`: `messages.db` normally, `demo.db` in demo mode. Both are readable only by you. The helper opens no network port, runs no system service, and never logs message text, contacts or credentials.

## Keyboard shortcuts

The full table, with every context, is in [docs/plan.md](docs/plan.md). The shortcuts that work anywhere in the window:

- `j` / `k`: move through conversations, or scroll the open one
- `Enter`: open the selected conversation, or send from the composer
- `Esc`: step back — close help or a dialog, clear search, leave the composer or the open conversation, then hide the window
- `Ctrl+K`: search conversations
- `Ctrl+N`: new conversation
- `Ctrl+1` / `Ctrl+2` / `Ctrl+0`: WhatsApp / Telegram / all conversations
- `F1` or `?`: show the shortcuts
- `r`: retry a failed message
- `m`: mute or unmute the open chat
- `u`: jump to the next unread conversation
- `q`: hide the window

## Helper API

Omarchy's shell starts the helper and talks to it over its stdin and stdout with [JSON-RPC 2.0](https://www.jsonrpc.org/specification), one JSON object per line. The helper exits when stdin closes or on SIGTERM.

```sh
oma-messenger-service [--demo] [--no-chatter] [--seed N] [--data-dir DIR] [--db FILE] [--version]
```

### Methods

| Method | Params | Result |
| --- | --- | --- |
| `hello` | | `{protocol, version, demo, unreadTotal}` |
| `accounts.list` | | `[Account]` |
| `contacts.list` | `{accountId, query}` | `[Contact]` |
| `conversations.list` | `{query}` | `[Conversation]`, newest first; `match` holds the newest matching message |
| `conversations.open` | `{accountId, contactId}` | `Conversation`, created if needed |
| `conversations.markRead` | `{conversationId}` | `{}` |
| `conversations.setMuted` | `{conversationId, muted}` | `Conversation` |
| `messages.list` | `{conversationId, before, limit}` | `{messages, hasMore}`, oldest first; `limit` 1–200, default 50 |
| `messages.send` | `{conversationId, text}` | `Message`; status `failed` if the service refused it |
| `messages.retry` | `{messageId}` | `Message`; only for failed outgoing messages |
| `ui.setFocus` | `{conversationId, windowActive}` | `{}` |
| `settings.apply` | `{notifications, notificationPreview, demoChatter}` | `{}` |
| `demo.inject` | `{conversationId}` | `Message`; demo mode only |

The protocol version is `2`.

### Events

Events are JSON-RPC notifications: `{"jsonrpc":"2.0","method":"<event>","params":<data>}`.

| Event | Data |
| --- | --- |
| `account.updated` | `Account` |
| `conversation.updated` | `Conversation` |
| `message.added` | `Message` |
| `message.updated` | `Message` |
| `unread.changed` | `{total}` |
| `typing` | `{conversationId, name, active}` |

### Errors

| Code | Meaning |
| --- | --- |
| `-32602` | Invalid params or input. The message says what to fix and is safe to show. |
| `-32601` | Unknown method, for example `demo.inject` outside demo mode. |
| `-32001` | The account, contact, conversation or message does not exist. |
| `-32603` | Internal error. Details stay in the helper. |

The shapes of `Account`, `Contact`, `Conversation` and `Message` are the JSON fields in [backend/internal/domain/domain.go](backend/internal/domain/domain.go).

## Development

You need Go 1.26+, Node 22+ and, for QML lint, Omarchy's shell and Qt 6.

```sh
make help            # list the commands
make check           # every gate: build, tests with coverage, lint
make build           # build the helper into bin/dev/, which the launcher prefers
make install-local   # install this checkout into Omarchy and enable it
go run ./backend --demo --no-chatter --seed 1 --data-dir "$(mktemp -d)"
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to contribute and [AGENTS.md](AGENTS.md) for the project rules.

### Releasing

Binaries are never committed. To release:

1. Change `helper-version` and `helperVersion` in `backend/config.go` together; a test checks they match.
2. Push a matching tag such as `v0.3.0`.

The release workflow tests the helper, builds both binaries, records a build-provenance attestation and publishes a GitHub Release with `SHA256SUMS`.

## Layout

- `backend/`: the Go helper. `internal/domain` (shared types), `store` (SQLite), `connector` (the service boundary, with `demo`), `app` (what the client does), `server` (JSON-RPC), `notify` (desktop notifications)
- `ui/`: the QML UI: `theme/` (typed Omarchy tokens), `components/` (views), `lib/` (pure JavaScript, tested with node), `Service.qml`, `Panel.qml`, `BarWidget.qml`
- `manifest.json`: the plugin manifest, pointing at the entry points in `ui/`
- `bin/oma-messenger-service`: launcher that runs `bin/dev/` if built, otherwise the installed release
- `scripts/`: helper install, release build, local plugin install, QML lint imports
- `tools/nologcontent`: a Go analyzer that keeps message content out of logs
- `tests/unit/`: JavaScript tests for `ui/lib` and the scripts
- `docs/plan.md`, `docs/decisions.md`: the build plan and the reasons behind design choices
