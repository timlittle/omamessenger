# OmaMessenger

OmaMessenger is a keyboard-first messaging client for [Omarchy](https://omarchy.org), for WhatsApp and Telegram. It runs as an Omarchy plugin: the window is a normal Quickshell window inside `omarchy-shell`, and a small Go helper keeps the message database and talks to the messaging services.

## Status

- **Helper and UI:** complete and tested against scripted fake accounts, which only test builds contain: sending with delivery receipts, failures and retries, unread counts, notifications, search, the command palette and keyboard navigation.
- **Telegram:** the helper connects through [gotd/td](https://github.com/gotd/td): sign-in by QR code, or phone and code with two-step verification, then recent chats, live messages, sending, read receipts, and pin and archive kept in step with the app. Add an account from the window; see below.
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

Installing the helper also adds OmaMessenger to Omarchy's apps menu (**SUPER+ALT+SPACE**), so you can launch it from there like any other app. To bind it to a key instead, add a line to `~/.config/hypr/bindings.lua`:

```lua
o.bind("SUPER + ALT + M", "OmaMessenger", "omarchy-shell shell summon io.github.omamessenger '{}'")
```

The apps-menu entry is `~/.local/share/applications/io.github.omamessenger.desktop`; removing the plugin does not remove it, so delete that file yourself if you want it gone.

## Add your Telegram account

Choose **Add an account** in the window (or in the command palette), then scan the QR code from Telegram on your phone (**Settings → Devices → Link Desktop Device**). With more than one messaging service available, a small chooser asks which one first. Or choose **Use phone number instead** and enter the code Telegram sends you. If the account has two-step verification, enter its password. Your recent chats appear once it connects, and the session stays on this computer. To remove an account, choose **Remove an account** in the command palette: it signs the account out and deletes its chats from this computer.

OmaMessenger signs in as its own Telegram app, whose API id and hash are in the source like any Telegram client's. To use an app of your own instead, register one at [my.telegram.org](https://my.telegram.org/apps) and choose **Add a Telegram account with your own API keys** in the command palette.

## Data and privacy

The helper keeps its data in `${XDG_DATA_HOME:-~/.local/share}/omamessenger/`, readable only by you:

- `messages.db` — your accounts, chats and messages
- `telegram/` — each Telegram account's login session and the API keys it signs in with
- `media/` — photos and files you have opened, cached up to 1 GB with the least recently used dropped first, plus `media/outgoing/`, where a file you attach to an outgoing message is kept so a retry can resend it
- `~/.local/share/applications/io.github.omamessenger.desktop` — the apps-menu entry `scripts/install-helper.sh` creates; removing the plugin does not remove this file

Nothing here ever leaves this machine except the message text and files you actually send, which go to Telegram (or WhatsApp, once supported) as any client's would. The helper opens no network port, runs no system service, and never logs message text, contacts or credentials.

Removing an account (**Remove an account** in the command palette) signs it out and deletes its session, credentials and messages from this computer, but it does not delete anything from your phone or the messaging service: your chat history there is unaffected, and other devices stay linked until you unlink them yourself (in Telegram, **Settings → Devices**).

## Risks

OmaMessenger is an unofficial Telegram client, not something Telegram publishes or endorses. It signs in with its own API id and hash, committed in the source like any open-source client's (`backend/internal/connector/telegram/credentials.go`): anyone can read and reuse them, since they are not a secret, only an identifier. Telegram can rate-limit, restrict or ban an account it decides is misusing an unofficial client, independent of anything OmaMessenger itself does wrong. Using OmaMessenger with your Telegram account is a risk you take on for that account; register your own API keys at [my.telegram.org](https://my.telegram.org/apps) (see above) if you would rather Telegram's limits apply to an app you control instead.

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
| Esc | Step back: close the palette, a dialog, account setup or the photo viewer, clear the search, leave the composer or the conversation |

The list shows chats from the last month, plus any with unread messages or chats you have open; everything else — older chats, ones you archived with the service, and ones you hid yourself — stays out of the way. The line under the list, or **Show or hide all chats** in the command palette, reveals the rest: those chats appear slightly dimmed, each labelled "Hidden" or "Archived" so the reason is never just a shade of colour, while the chats you look at every day stay easy to pick out. Sending a message in an older chat brings it back into the standard list, since its activity is now recent; sending in a hidden chat leaves it hidden until you unhide it yourself. An unread message still arrives and notifies as usual even while its chat is hidden. Search always looks through every chat, including hidden and archived ones: it matches a chat's title as substring text, and message bodies word by word as a prefix, ignoring case and accents, so "cafe" finds "café" and "tick" finds "ticket". Pinned chats always lead the list, with a pin mark; an archived chat's unread badge is dimmed like a muted one's, since it is already filed away, though it still counts towards the unread totals in the rail.

In the conversation list, `j` / `k` move, `Enter` opens, and `m` mutes; **Pin or unpin chat**, **Archive or unarchive chat** and **Hide or unhide chat** are in the command palette. Hiding a chat is local to OmaMessenger only: it is never sent to WhatsApp or Telegram, and it never leaves this computer. In a conversation, `j` / `k` scroll, `i` or `Enter` starts writing, `h` goes back to the list, `r` retries a failed message, `R` replies to the newest message, and **React to the newest message** opens a short emoji picker; a small toolbar also appears over the top corner of a bubble when you hover it, with a `+` that opens the same picker and a reply button, without moving any message. While writing, `Enter` sends; `Shift+Enter` or `Ctrl+J` starts a new line instead, and the composer grows to show it, up to six lines, then scrolls. The composer shows "Replying to …" with a way to cancel it: `Esc`, or the `✕` beside it; sending clears it. A reply's quote in the bubble above its text scrolls to the original message when clicked, if it is loaded. Clicking a chip toggles that reaction; in the picker, `←` / `→` move and `Enter` picks.

Clicking a photo opens it inside the window, as large as the window allows, rather than in your system's image viewer: Omarchy floats that viewer too small to reach with the keyboard, and focus stays on OmaMessenger, so a photo opened there would get stuck with no way to close it. While the full photo is still downloading it shows the same blurred preview the message bubble does. Esc, the ✕ button or a click outside the photo closes it; ← / → steps to the previous or next photo in the conversation; and **Open in image viewer** opens it in your own application instead. Videos and files still open externally, as before.

To send a photo or file, click the composer's attach button (📎) or choose **Attach a file** in the command palette to pick one, or paste an image with **Ctrl+V** while writing; pasting text still works as before. The picked or pasted attachment shows as a chip above the text field with a ✕ to remove it; `Esc` removes it too, before leaving the composer.

## Helper API

Omarchy's shell starts the helper and talks to it over its stdin and stdout with [JSON-RPC 2.0](https://www.jsonrpc.org/specification), one JSON object per line. The helper exits when stdin closes or on SIGTERM.

```sh
oma-messenger-service [--data-dir DIR] [--db FILE] [--version]
```

### Methods

| Method | Params | Result |
| --- | --- | --- |
| `hello` | | `{protocol, version, unreadTotal, services}`; `services` is `[{id, name}]`, the messaging services available to add an account for |
| `accounts.list` | | `[Account]` |
| `accounts.add` | `{service, apiId, apiHash, options}` | `Account`. `apiId` and `apiHash` are Telegram's own API keys, kept for compatibility; `options` is the general form, a map of a provider's own setup values (Telegram reads the same two keys from it as `apiId`/`apiHash` strings). Without either, the account uses OmaMessenger's own keys |
| `accounts.remove` | `{accountId}` | `{}`; signs out and deletes the account's session, credentials and messages |
| `auth.submit` | `{accountId, step, value}` | `{}`; answers an `auth.step`: `phone`, `code` or `password` |
| `contacts.list` | `{accountId, query}` | `[Contact]` |
| `conversations.list` | `{query}` | `[Conversation]`, newest first; `match` holds the newest matching message |
| `conversations.open` | `{accountId, contactId}` | `Conversation`, created if needed |
| `conversations.markRead` | `{conversationId}` | `{}` |
| `conversations.setMuted` | `{conversationId, muted}` | `Conversation` |
| `conversations.setPinned` | `{conversationId, pinned}` | `Conversation`; `conversations.list` always orders pinned conversations first |
| `conversations.setArchived` | `{conversationId, archived}` | `Conversation`; `conversations.list` still returns archived conversations, the UI hides them by default |
| `conversations.setHidden` | `{conversationId, hidden}` | `Conversation`; local to this computer only, never reported to the service. `conversations.list` still returns hidden conversations, the UI hides them by default |
| `messages.list` | `{conversationId, before, limit}` | `{messages, hasMore}`, oldest first; `limit` 1–200, default 50. Past the oldest stored message it fetches older history from the service, which arrives as `message.added` too |
| `messages.send` | `{conversationId, text, attachment, replyTo}` | `Message`; status `failed` if the service refused it. `attachment` is optional: `{path}` names a file on this machine to send, with `text` as its caption (`text` may then be empty); files over 2 GB are rejected. `replyTo`, also optional, is the local id of a message in the same conversation this one answers; it works together with `attachment` |
| `messages.retry` | `{messageId}` | `Message`; only for failed outgoing messages |
| `messages.react` | `{messageId, emoji}` | `Message`; sets the user's reaction to `emoji`, or clears it when `emoji` is `""`. Fails with invalid params if the service does not support reactions |
| `media.fetch` | `{messageId}` | `{path}`: the message's photo, video or file. For a message you sent, its own local copy is returned at once; otherwise it is downloaded into the media cache the first time |
| `media.paste` | | `{path, kind, width, height}`: an image copied off the clipboard into the outgoing media area, for the composer to attach to the next message sent; fails with an invalid-input error when the clipboard holds no image |
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
| `message.updated` | `Message`; also sent when the service reports a message edited, or its reactions changed |
| `message.removed` | `{conversationId, messageId}`; sent when the service reports a message deleted |
| `unread.changed` | `{total}` |
| `typing` | `{conversationId, name, active}` |
| `notification.clicked` | `{conversationId}`: the user clicked a desktop notification; the UI opens that conversation |

A `Message` may carry `media`: `{kind, …}` where `kind` is `link` (with `url`, `siteName`, `title`, `description`), `photo`, `video` or `file`. `thumb` is a small base64 JPEG preview sent with the message. `edited` is `true` once the service reports the message changed since it was first sent. A `Message` may also carry `replyTo`: `{remoteId, senderName, text}`, the message it answers, with `senderName` and `text` (a short excerpt) filled in once the quoted message is known locally. `reactions` is `[{emoji, count, mine}]`, omitted when the message has none; a custom-emoji reaction Telegram sends is left out rather than shown as a misleading placeholder.

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

### Adding a messaging service

Every service plugs into the same boundary, so adding one means:

1. A connector package under `backend/internal/connector/<service>/` implementing `connector.Connector` (and whichever optional capabilities it supports: `Authenticator`, `HistoryLoader`, `MediaFetcher`, `MessageRefresher`, `Organizer`, `Reactor`).
2. A `Provider` in that package implementing `connector.Provider`, so the registry can prepare, connect and forget its accounts.
3. Registering it in `backend/main.go`'s `providers()`.
4. A glyph for it in `ui/components/ServiceGlyph.qml`.
5. A depguard rule for the new package in `.golangci.yml`.
6. Once it exists, the conformance suite in `backend/internal/connector/connectortest` run against the new connector.

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
