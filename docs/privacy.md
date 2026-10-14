# Data and privacy

The helper stores its data in `${XDG_DATA_HOME:-~/.local/share}/omamessenger/`: `messages.db` (accounts, chats and messages), `telegram/` (each account's session and API keys), `whatsapp/` (each account's session and the media references it needs to download photos and files later), `media/` (cached photos and files) and `spelling/` (your personal spell-check dictionary), all `0600`, their directories `0700`.

OmaMessenger is an unofficial Telegram client: it uses Telegram's own API with your account, and Telegram can rate-limit, restrict or ban an account independent of anything OmaMessenger does. WhatsApp support uses an unofficial library ([whatsmeow](https://github.com/tulir/whatsmeow)) the same way, and WhatsApp may similarly restrict or ban an account for using an unofficial client. The helper connects only to the messaging services. It does not log credentials, session keys, phone numbers, message text or the words a spell check looks up.

## What OmaMessenger reads, writes and sends

| What | Where | Mode | Why |
| --- | --- | --- | --- |
| Accounts, chats and messages | `$XDG_DATA_HOME/omamessenger/messages.db` | file `0600`, directory `0700` | the local message database |
| Telegram sessions and API keys | `$XDG_DATA_HOME/omamessenger/telegram/` | files `0600`, directory `0700` | signed-in Telegram accounts |
| WhatsApp sessions and media references | `$XDG_DATA_HOME/omamessenger/whatsapp/` | files `0600`, directory `0700` | signed-in WhatsApp accounts |
| Cached photos, video and files | `$XDG_DATA_HOME/omamessenger/media/` | files `0600`, directory `0700` | avoids re-downloading media already seen |
| Your personal spell-check dictionary | `$XDG_DATA_HOME/omamessenger/spelling/` | files `0600`, directory `0700` | words added to the dictionary through the composer's spelling picker |
| The installed helper binary | `$XDG_DATA_HOME/omamessenger/bin/` | executable, directory `0700` | the helper release pinned in `helper-version` |
| Plugin source (this checkout) | `~/.config/omarchy/plugins/io.github.omamessenger/` | as installed, read-only at runtime | the QML UI and manifest Omarchy loads; no session or message data is ever written here |
| Apps menu entry | `~/.local/share/applications/io.github.omamessenger.desktop` | `0644`, no secrets | lets Omarchy's menu and **SUPER+ALT+SPACE** open OmaMessenger |
| Network requests | Telegram's and WhatsApp's own servers; `github.com/timlittle/omamessenger/releases` only to download the helper the first time the window opens | — | no other network access |
| Diagnostics | stderr, which `omarchy-shell` sends to the system journal | — | states, counts and safe categories only, never credentials, QR tokens, session keys, phone numbers or message text |

Run `oma-messenger-service doctor`, or **Run health check** in the command palette, to check the data directory's permissions, the database, the media cache, the outgoing media area, each account's connection and whether a notification service is reachable, without exposing any of their contents: every line it prints is a state or a category, never a path, a name or a token, except for a plain count where it helps, such as how many failed messages are waiting in an over-limit outgoing media area.

WhatsApp chats show the recent history your phone syncs when you link the device; scrolling back further asks your phone for more, so it needs to be online. Telegram loads more on demand.

Removing an account (**Remove an account** in the command palette) signs it out and deletes its session and messages from this computer. It does not touch your chat history on the service or on other devices.

### Reading and replying without the sender being told

Turning off read receipts (**Toggle read receipts** in the command palette, or the plugin's own "Read receipts" setting) only ever affects what this account tells the service: a chat read here still clears its own badge locally, but Telegram's or WhatsApp's read receipt is never sent, so the sender, and this account's own other devices, keep seeing it as unread. A quiet "Read receipts off" label shows in the footer while it is on.

This is the one signal this app ever sends about a chat being looked at, so turning it off is a complete "read without being seen reading" mode, not a partial one. The rest of this section is the audit behind that claim, so it is a documented fact rather than an assumption:

- **Typing indicators.** OmaMessenger shows the other side's typing indicator to you (the `typing` event), but it never sends your own: there is no code path, in either connector, that reports you are composing a reply. Nothing needed turning off.
- **Online/presence.** Neither connector ever calls WhatsApp's "available"/"unavailable" presence or Telegram's `account.updateStatus`; OmaMessenger has no notion of "online" to report at all. What it cannot control: simply being connected can itself be visible. A linked WhatsApp device and an active Telegram session can both surface as "online" or a recent "last seen" to someone who has your chat open, as part of how each service's own protocol works, regardless of anything this app sends or withholds.
- **Played receipts for voice notes.** Opening or playing a voice note here never sends a distinct "played" signal: WhatsApp's read receipt call here is never given its separate played-receipt type, and Telegram's own `messages.readMessageContents` (its equivalent for a voice note or round video) is never called either. A voice note gets exactly the same, single read receipt as a text message — covered by the one setting above like everything else, with nothing extra to turn off.
- **Downloading media.** Fetching a photo, video or file to view it is a download from the service's media storage; it is not a report that you opened the message, and nothing about it reaches the sender.
- **Replying.** Sending a reply at all is not something this setting touches or could: the message itself reaches the service and the other side, the same as always, which necessarily shows you active regardless of the read-receipts setting. Turn read receipts back on afterwards if you want the chat's own unread state to resync normally.

These were checked directly in the WhatsApp and Telegram connectors' own code, each backed by a test (`TestMarkRead_SendsNoOtherSignal` in both `backend/internal/connector/whatsapp` and `backend/internal/connector/telegram`) that fails if a future change ever adds one of these calls outside this setting's gate.

Spell check runs entirely on this computer, against a dictionary bundled with the helper: the text you are composing never leaves it, and nothing about spell check reaches Telegram, WhatsApp or anywhere else. Turning it off in the plugin's "Spell check" setting releases the dictionary from memory until it is turned back on.
