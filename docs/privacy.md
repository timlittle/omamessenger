# Data and privacy

The helper stores its data in `${XDG_DATA_HOME:-~/.local/share}/omamessenger/`: `messages.db` (accounts, chats and messages), `telegram/` (each account's session and API keys), `whatsapp/` (each account's session and the media references it needs to download photos and files later) and `media/` (cached photos and files), all `0600`, their directories `0700`.

OmaMessenger is an unofficial Telegram client: it uses Telegram's own API with your account, and Telegram can rate-limit, restrict or ban an account independent of anything OmaMessenger does. WhatsApp support uses an unofficial library ([whatsmeow](https://github.com/tulir/whatsmeow)) the same way, and WhatsApp may similarly restrict or ban an account for using an unofficial client. The helper connects only to the messaging services. It does not log credentials, session keys, phone numbers or message text.

## What OmaMessenger reads, writes and sends

| What | Where | Mode | Why |
| --- | --- | --- | --- |
| Accounts, chats and messages | `$XDG_DATA_HOME/omamessenger/messages.db` | file `0600`, directory `0700` | the local message database |
| Telegram sessions and API keys | `$XDG_DATA_HOME/omamessenger/telegram/` | files `0600`, directory `0700` | signed-in Telegram accounts |
| WhatsApp sessions and media references | `$XDG_DATA_HOME/omamessenger/whatsapp/` | files `0600`, directory `0700` | signed-in WhatsApp accounts |
| Cached photos, video and files | `$XDG_DATA_HOME/omamessenger/media/` | files `0600`, directory `0700` | avoids re-downloading media already seen |
| The installed helper binary | `$XDG_DATA_HOME/omamessenger/bin/` | executable, directory `0700` | the helper release pinned in `helper-version` |
| Plugin source (this checkout) | `~/.config/omarchy/plugins/io.github.omamessenger/` | as installed, read-only at runtime | the QML UI and manifest Omarchy loads; no session or message data is ever written here |
| Apps menu entry | `~/.local/share/applications/io.github.omamessenger.desktop` | `0644`, no secrets | lets Omarchy's menu and **SUPER+ALT+SPACE** open OmaMessenger |
| Network requests | Telegram's and WhatsApp's own servers; `github.com/timlittle/omamessenger/releases` only to download the helper the first time the window opens | — | no other network access |
| Diagnostics | stderr, which `omarchy-shell` sends to the system journal | — | states, counts and safe categories only, never credentials, QR tokens, session keys, phone numbers or message text |

Run `oma-messenger-service doctor`, or **Run health check** in the command palette, to check the data directory's permissions, the database, the media cache, the outgoing media area, each account's connection and notify-send availability without exposing any of their contents: every line it prints is a state or a category, never a path, a name or a token, except for a plain count where it helps, such as how many failed messages are waiting in an over-limit outgoing media area.

WhatsApp chats show the recent history your phone syncs when you link the device; scrolling back further asks your phone for more, so it needs to be online. Telegram loads more on demand.

Removing an account (**Remove an account** in the command palette) signs it out and deletes its session and messages from this computer. It does not touch your chat history on the service or on other devices.

Turning off read receipts (**Toggle read receipts** in the command palette, or the plugin's own "Read receipts" setting) only ever affects what this account tells the service: a chat read here still clears its own badge locally, but Telegram's or WhatsApp's read receipt is never sent, so the sender, and this account's own other devices, keep seeing it as unread. A quiet "Read receipts off" label shows in the footer while it is on.
