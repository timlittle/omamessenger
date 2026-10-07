# OmaMessenger

A keyboard-first Telegram and WhatsApp client for [Omarchy](https://omarchy.org).

![OmaMessenger](docs/demo.gif)

OmaMessenger is an Omarchy plugin. The window runs inside `omarchy-shell`, managed by Hyprland like any other app, backed by a small Go helper that talks to Telegram's API. WhatsApp is planned, not yet connected.

## Features

- A unified conversation list across every connected account, with a rail to filter by service
- Unread badges, search across every conversation (including archived ones), and older history that loads as you scroll back
- Photos, video and files, with an in-window photo viewer and link previews
- Replies and reactions; edits and deletes sync from the service
- Pin, archive and a local-only hide, with a one-month recency filter and a "show all" to see everything
- Desktop notifications; click one to open its conversation
- Paste or attach an image to a message
- Keyboard-first, with a command palette (**Ctrl+/**) listing every action and its shortcut

Telegram sign-in works today, by QR code or by phone number and code, with two-step verification. WhatsApp support is planned.

## Install

```sh
omarchy plugin add https://github.com/timlittle/omamessenger --enable
```

Then install the helper binary, either from the window's **Install helper** button or from a terminal:

```sh
~/.config/omarchy/plugins/io.github.omamessenger/scripts/install-helper.sh
```

This downloads the release pinned in `helper-version`, checks it against that release's `SHA256SUMS`, and confirms it reports the right version. Nothing is downloaded until you run it. No release has been published yet, so until `v0.3.0` is tagged, build the helper from a checkout instead (`make build`; see Development below).

Installing the helper adds OmaMessenger to Omarchy's apps menu (**SUPER+ALT+SPACE**). To bind it to a key instead, add a line to `~/.config/hypr/bindings.lua`:

```lua
o.bind("SUPER + ALT + M", "OmaMessenger", "omarchy-shell shell summon io.github.omamessenger '{}'")
```

## Sign in to Telegram

Choose **Add an account** in the window or the command palette. Scan the QR code shown with Telegram (**Settings → Devices → Link Desktop Device**), or choose **Use phone number instead** for a code, and a password if the account has two-step verification. Recent chats sync once it connects.

OmaMessenger signs in with its own Telegram API id and hash, committed in the source like any open-source Telegram client's. To use an app of your own instead, register one at [my.telegram.org](https://my.telegram.org/apps) and choose **Add a Telegram account with your own API keys** in the command palette.

## Keyboard shortcuts

**Ctrl+/** opens the command palette, which lists every command with its shortcut.

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

In the conversation list: `j` / `k` move, `Enter` (or `i`, `l`, `o`) opens, `m` mutes. Pin, archive and hide are in the command palette, and are local to this computer only.

In a conversation: `j` / `k` scroll, `i` or `Enter` starts writing, `h` goes back to the list, `r` retries a failed message, `R` replies to the newest message. The composer shows which mode you are in: a quiet border with an "i to write" hint while scrolling, an accent border with a "Writing · Esc to stop" hint once it has focus. While writing, `Enter` sends; `Shift+Enter` or `Ctrl+J` starts a new line instead. `Ctrl+V` pastes a clipboard image as an attachment.

Clicking a photo opens it inside the window, sized to fit; `←` / `→` steps through the conversation's photos, and **Open in image viewer** opens it in your own application. Videos and files still open externally.

## Data and privacy

The helper stores its data in `${XDG_DATA_HOME:-~/.local/share}/omamessenger/`: `messages.db` (accounts, chats and messages), `telegram/` (each account's session and API keys) and `media/` (cached photos and files), all `0600`, their directories `0700`.

OmaMessenger is an unofficial Telegram client: it uses Telegram's own API with your account, and Telegram can rate-limit, restrict or ban an account independent of anything OmaMessenger does. Message text and files you send go to Telegram (or WhatsApp, once supported), as with any client. The helper makes no other network requests and never logs credentials, QR tokens, session keys, phone numbers or message bodies.

Removing an account (**Remove an account** in the command palette) signs it out and deletes its session and messages from this computer. It does not touch your chat history on the service or on other devices.

## Uninstall

```sh
omarchy plugin remove io.github.omamessenger
rm -rf ~/.local/share/omamessenger
rm -f ~/.local/share/applications/io.github.omamessenger.desktop
```

Also remove any `o.bind(...)` line you added to `~/.config/hypr/bindings.lua`.

## Helper API

The window talks to the helper over its stdin and stdout with JSON-RPC 2.0, one JSON object per line. The method and event reference is in [docs/api.md](docs/api.md).

## Development

Needs Go 1.26+, Node 22+, and, for QML lint, Omarchy's shell and Qt 6.

```sh
make help            # list the commands
make check           # every gate: build, tests with coverage, lint
make build           # build the helper into bin/dev/, which the launcher prefers
make install-local   # install this checkout into Omarchy and enable it
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the contribution process and [AGENTS.md](AGENTS.md) for the project rules.

## License

MIT, see [LICENSE](LICENSE).
