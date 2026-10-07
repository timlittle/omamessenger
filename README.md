# OmaMessenger

OmaMessenger is a keyboard-first messaging client for [Omarchy](https://omarchy.org), for WhatsApp and Telegram. It runs as an Omarchy plugin: the window is a normal Quickshell window inside `omarchy-shell`, and a small Go helper keeps the message database and talks to the messaging services.

## Status

- **Helper and UI:** complete and tested against scripted fake accounts, which only test builds contain: sending with delivery receipts, failures and retries, unread counts, notifications, search, the command palette and keyboard navigation.
- **Telegram:** the helper connects through [gotd/td](https://github.com/gotd/td): sign-in by QR code, or phone and code with two-step verification, then recent chats, live messages, sending and read receipts. Add an account from the window; see below.
- **WhatsApp:** not connected yet. The planned library is [whatsmeow](https://github.com/tulir/whatsmeow).

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

## Add your Telegram account

Choose **Add Telegram account** in the window (or **Add a Telegram account** in the command palette), then scan the QR code from Telegram on your phone (**Settings → Devices → Link Desktop Device**). Or choose **Use phone number instead** and enter the code Telegram sends you. If the account has two-step verification, enter its password. Your recent chats appear once it connects, and the session stays on this computer. To remove an account, choose **Remove an account** in the command palette: it signs the account out and deletes its chats from this computer.

OmaMessenger signs in as its own Telegram app, whose API id and hash are in the source like any Telegram client's. To use an app of your own instead, register one at [my.telegram.org](https://my.telegram.org/apps) and choose **Add a Telegram account with your own API keys** in the command palette.

## Data and privacy

The helper keeps its data in `${XDG_DATA_HOME:-~/.local/share}/omamessenger/`, readable only by you: `messages.db`, and in `telegram/` each Telegram account's session and the API keys it signs in with. The helper opens no network port, runs no system service, and never logs message text, contacts or credentials.

## Keyboard shortcuts

The shortcuts follow Slack's. **Ctrl+/** opens the command palette, which lists every command with its shortcut, so you can find anything there and learn the keys as you go.

| Keys | Does |
| --- | --- |
| Ctrl+/ or Ctrl+Shift+P | Command palette |
| Ctrl+K or Ctrl+T | Jump to a conversation |
| Ctrl+G | Search messages |
| Ctrl+N or Ctrl+Shift+K | New message |
| Ctrl+J, Alt+Shift+↓ / ↑ | Next / previous unread conversation |
| Alt+↓ / ↑ | Next / previous conversation |
| Ctrl+0 / 1 / 2 | All / WhatsApp / Telegram |
| Ctrl+Tab, Ctrl+Shift+Tab | Next / previous account or service |
| Ctrl+W | Close the window (asks whether to keep running) |
| Ctrl+Q | Quit |
| Esc | Step back: close the palette, a dialog or account setup, clear the search, leave the composer or the conversation |

The list shows chats from the last month, plus any with unread messages; the line under it, or **Show or hide chats older than a month** in the command palette, shows the rest. Search always looks through every chat.

In the conversation list, `j` / `k` move, `Enter` opens, and `m` mutes. In a conversation, `j` / `k` scroll, `i` or `Enter` starts writing, `h` goes back to the list, and `r` retries a failed message.

## Helper API

Omarchy's shell starts the helper and talks to it over its stdin and stdout with [JSON-RPC 2.0](https://www.jsonrpc.org/specification), one JSON object per line. The helper exits when stdin closes or on SIGTERM.

```sh
oma-messenger-service [--data-dir DIR] [--db FILE] [--version]
```

### Methods

| Method | Params | Result |
| --- | --- | --- |
| `hello` | | `{protocol, version, unreadTotal}` |
| `accounts.list` | | `[Account]` |
| `accounts.add` | `{service, apiId, apiHash}` | `Account`; Telegram only. `apiId` and `apiHash` are optional: without them the account uses OmaMessenger's own Telegram app |
| `accounts.remove` | `{accountId}` | `{}`; signs out and deletes the account's session, credentials and messages |
| `auth.submit` | `{accountId, step, value}` | `{}`; answers an `auth.step`: `phone`, `code` or `password` |
| `contacts.list` | `{accountId, query}` | `[Contact]` |
| `conversations.list` | `{query}` | `[Conversation]`, newest first; `match` holds the newest matching message |
| `conversations.open` | `{accountId, contactId}` | `Conversation`, created if needed |
| `conversations.markRead` | `{conversationId}` | `{}` |
| `conversations.setMuted` | `{conversationId, muted}` | `Conversation` |
| `messages.list` | `{conversationId, before, limit}` | `{messages, hasMore}`, oldest first; `limit` 1–200, default 50. Past the oldest stored message it fetches older history from the service, which arrives as `message.added` too |
| `messages.send` | `{conversationId, text}` | `Message`; status `failed` if the service refused it |
| `messages.retry` | `{messageId}` | `Message`; only for failed outgoing messages |
| `ui.setFocus` | `{conversationId, windowActive}` | `{}` |
| `settings.apply` | `{notifications, notificationPreview}` | `{}` |
| `fake.inject` | `{conversationId}` | `Message`; only in the test build |

The protocol version is `3`.

### Events

Events are JSON-RPC notifications: `{"jsonrpc":"2.0","method":"<event>","params":<data>}`.

| Event | Data |
| --- | --- |
| `account.updated` | `Account`; `status` is `connecting`, `connected`, `needs-auth`, `error` or `offline` |
| `account.removed` | `{accountId}` |
| `auth.step` | `{accountId, kind, qr, hint}`: `kind` is `qr` (a base64 PNG to scan; reply with a `phone` to sign in by code instead), `code` or `password` |
| `conversation.updated` | `Conversation` |
| `message.added` | `Message` |
| `message.updated` | `Message` |
| `unread.changed` | `{total}` |
| `typing` | `{conversationId, name, active}` |

### Errors

| Code | Meaning |
| --- | --- |
| `-32602` | Invalid params or input. The message says what to fix and is safe to show. |
| `-32601` | Unknown method, for example `fake.inject` outside the test build. |
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
go run -tags fake ./backend --data-dir "$(mktemp -d)"   # the test build, with fake accounts
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to contribute and [AGENTS.md](AGENTS.md) for the project rules.

### Releasing

Binaries are never committed. To release:

1. Change `helper-version` and `helperVersion` in `backend/config.go` together; a test checks they match.
2. Push a matching tag such as `v0.3.0`.

The release workflow tests the helper, builds both binaries, records a build-provenance attestation and publishes a GitHub Release with `SHA256SUMS`.

## Layout

- `backend/`: the Go helper. `internal/domain` (shared types), `store` (SQLite), `connector` (the service boundary, with `fake` accounts for tests), `app` (what the client does), `server` (JSON-RPC), `notify` (desktop notifications)
- `ui/`: the QML UI: `theme/` (typed Omarchy tokens), `components/` (views), `lib/` (pure JavaScript, tested with node), `Service.qml`, `Panel.qml`, `BarWidget.qml`
- `manifest.json`: the plugin manifest, pointing at the entry points in `ui/`
- `bin/oma-messenger-service`: launcher that runs `bin/dev/` if built, otherwise the installed release
- `scripts/`: helper install, release build, local plugin install, QML lint imports
- `tools/nologcontent`: a Go analyzer that keeps message content out of logs
- `tests/unit/`: JavaScript tests for `ui/lib` and the scripts
- `docs/plan.md`, `docs/decisions.md`: the build plan and the reasons behind design choices
