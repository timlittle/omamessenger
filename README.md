# OmaMessenger

A keyboard-first messaging client for [Omarchy](https://omarchy.org). Telegram today, WhatsApp planned.

![OmaMessenger](docs/demo.gif)

OmaMessenger is an Omarchy plugin. The window runs inside `omarchy-shell`, and Hyprland manages it like any other app; a Go helper it starts handles the messaging underneath.

## Features

- A unified conversation list across every connected account, with a rail to filter by service
- Unread badges, search across every conversation (including archived ones), and older history that loads as you scroll back
- Photos, video and files, with an in-window photo viewer and link previews
- Replies and reactions; edits and deletes sync from the service
- Pin, archive and a local-only hide, with a one-month recency filter and a "show all" to see everything
- Desktop notifications; click one to open its conversation
- Paste or attach an image to a message
- Keyboard-first, with a command palette (**Ctrl+/**) listing every action and its shortcut
- Light on memory: about 100 MB in use, against about 1 GB for Telegram Desktop and WhatsApp web running side by side

## Install

```sh
omarchy plugin add https://github.com/timlittle/omamessenger --enable
```

The first time you open the window, it downloads and verifies the helper release pinned in `helper-version`, then starts it.

It also adds OmaMessenger to Omarchy's apps menu (**SUPER+ALT+SPACE**). To bind it to a key instead, add a line to `~/.config/hypr/bindings.lua`:

```lua
o.bind("SUPER + ALT + M", "OmaMessenger", "omarchy-shell shell summon io.github.omamessenger '{}'")
```

## Sign in to Telegram

Choose **Add an account** in the window or the command palette. Scan the QR code shown with Telegram (**Settings → Devices → Link Desktop Device**), or choose **Use phone number instead** for a code, and a password if the account has two-step verification. Recent chats sync once it connects.

To sign in with your own Telegram API keys instead of OmaMessenger's, see [docs/telegram.md](docs/telegram.md).

## Keyboard shortcuts

| Keys | Does |
| --- | --- |
| Ctrl+/ | Command palette |
| Ctrl+K | Jump to a conversation |
| Ctrl+N | New message |
| Ctrl+G | Search messages |
| Enter | Send (while writing) |
| Esc | Step back |
| Ctrl+W | Close the window |
| Ctrl+Q | Quit |

Ctrl+/ lists every command and its shortcut. The full reference, including the conversation list, composer and photo viewer, is in [docs/shortcuts.md](docs/shortcuts.md).

## Data and privacy

The helper stores its data in `${XDG_DATA_HOME:-~/.local/share}/omamessenger/`: `messages.db` (accounts, chats and messages), `telegram/` (each account's session and API keys) and `media/` (cached photos and files), all `0600`, their directories `0700`.

OmaMessenger is an unofficial Telegram client: it uses Telegram's own API with your account, and Telegram can rate-limit, restrict or ban an account independent of anything OmaMessenger does. The helper connects only to the messaging services. It does not log credentials, session keys, phone numbers or message text.

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

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup and contribution process, and [AGENTS.md](AGENTS.md) for the project rules.

## License

MIT, see [LICENSE](LICENSE).
