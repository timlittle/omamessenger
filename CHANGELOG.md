# Changelog

## [0.3.3](https://github.com/timlittle/omamessenger/compare/v0.3.2...v0.3.3) (2026-10-09)


### Bug Fixes

* **ui:** render text from other people as plain text so it cannot load remote images ([b8d0b1c](https://github.com/timlittle/omamessenger/commit/b8d0b1c016fea73b2c17b64386afd4de086cff46))

## [0.3.2](https://github.com/timlittle/omamessenger/compare/v0.3.1...v0.3.2) (2026-10-09)


### Bug Fixes

* **notify:** send desktop notifications over D-Bus so message text never appears in process arguments ([f8f6aa9](https://github.com/timlittle/omamessenger/commit/f8f6aa9a9f8d0971c87ae06fe6437c5632c90f67))
* **whatsapp:** drop key-distribution-only messages instead of showing a placeholder ([52b69b2](https://github.com/timlittle/omamessenger/commit/52b69b2e61b4c1149df5160b5952af65630b06cb))

## [0.3.1](https://github.com/timlittle/omamessenger/compare/v0.3.0...v0.3.1) (2026-10-09)


### Bug Fixes

* create and keep the data directory private (0700) ([b4ced75](https://github.com/timlittle/omamessenger/commit/b4ced75b7a83adda5cc30f360ecdb60432722075))

## 0.3.0 — first release

The first tagged release: a keyboard-first unified messaging client for Omarchy, with Telegram and WhatsApp connectors.

### Features

- **Telegram sign-in and sync** — add an account by QR code or phone number and code, with two-step verification support; recent chats sync on connect, live messages, sends and read receipts stay in step with the app, and a restart or a long time offline catches up on whatever happened meanwhile instead of losing it
- **WhatsApp sign-in and sync** — add an account by QR code or phone number and an 8-character code, linking this as a companion device; every chat on the phone appears, recent ones with their history, and live messages, sends, receipts, media (including albums), reactions, replies, pin and archive, business messages' own text and "Message yourself" all stay in step with the phone, including a message deleted on either side
- **Media** — send and receive photos, videos and files; photos open in an in-window viewer sized to the window, with a blurred preview while the full image downloads; paste an image from the clipboard straight into the composer; once a sent attachment is confirmed delivered, its outgoing copy moves into the media cache instead of being kept twice
- **Voice notes** — play inline in the message bubble, with a scrubber and elapsed/total time; without `qt6-multimedia` installed, a voice note opens in your default player instead
- **Replies** — reply to any message, with a quote shown in the bubble that scrolls to the original when clicked
- **Reactions** — react to a message with a short emoji picker; one reaction per person, kept in step with Telegram and WhatsApp
- **Stickers and polls** — a sticker shows as a static image even when it is animated; a poll shows live results and can be voted on with the mouse or the keyboard (**v**); sending either is not yet supported
- **@-mentions** — typing "@" in a group conversation's composer opens a picker of its members; a message that mentions the signed-in account shows with a highlight
- **Deleting a message** — delete a message yourself (**d**), for everyone or just for you, including a failed send that never reached the service; a deletion made elsewhere syncs in too
- **Failed sends retry themselves** — a message that failed to send retries automatically once its account reconnects, and otherwise on a backoff, re-copying its attachment from the original file if the outgoing copy goes missing; a scheduled retry shows quietly beside "Not sent" as "Retrying in N min" or the time it will happen; **t** still retries sooner and starts the backoff over
- **Search** — **Ctrl+K** and **Ctrl+G** open the same palette: full-text search across every conversation, including archived ones, matching a chat's title as substring text and message bodies word by word as a prefix, ignoring case and accents
- **Incognito read receipts** — turn read receipts off from the command palette or the plugin's own setting, so a chat read here still clears locally but the service, and this account's other devices, keep showing it unread; a quiet "Read receipts off" label shows in the footer while it is on
- **Pin and archive** — pin or unpin, and archive or unarchive, a conversation, kept in step with the service; **a** archives and marks a conversation read in one step, and "Archive all read conversations" (command palette) archives every read, unpinned conversation in the current list after confirming
- **Snooze and reminders** — snooze a conversation until later today, tomorrow, next week or a custom time; it returns to the top of the list marked "Reminder" with a desktop notification once due, local to this computer only
- **Unread view** — **Ctrl+Shift+A** shows every unread conversation across every service and account in place of the usual list, and **Ctrl+K**'s jump-to-conversation list sorts unread conversations first
- **Keyboard navigation between conversations** — **Ctrl+J** and **Alt+↑/↓** move to the next or previous unread or any conversation, from the list, an open conversation or the unread view, keeping whichever mode — writing or scrolling — you were in
- **Hide and show all** — chats with no activity for a month fold out of the list, and you can hide a chat on this computer only; **Show all** brings both back, dimmed and labelled
- **History and link previews** — older messages load as you scroll back for both Telegram and WhatsApp (WhatsApp asks your phone, so it needs to be online), and links show a preview card
- **Tables** — Markdown tables in messages render as tables
- **Writing and scrolling modes** — the composer is dimmed while you scroll and shows which keys switch between the two
- **Keyboard-centric dialogs and message actions** — adding or removing an account, and the close-window question, all work from the keyboard with a visible highlight; a highlighted message can be opened (**Enter**), replied to (**r**), reacted to (**e**), have its link opened (**o**), jump to the message it replies to (**p**), retried if it failed (**t**), voted in its poll (**v**), or deleted (**d**)
- **Group chat senders** — each sender in a group chat gets a stable colour and an initials avatar on the first message of a run, so consecutive messages from different people are easy to tell apart
- **Account colours** — once more than one account is connected, each gets a stable colour tag shown in the conversation list and the rail, so chats from different accounts are easy to tell apart
- **Notifications** — desktop notifications for new messages, with three levels of detail to choose from (name and message, name only, or nothing); clicking a notification opens its conversation; messages that arrived while the helper was not running count as unread without a burst of notifications at startup
- **Keyboard and command palette** — every action reachable from the keyboard, Slack-style shortcuts, and a command palette (**Ctrl+/**) that lists every command with its shortcut
- **Key remapping** — override any default shortcut in `keys.conf`, created and opened from the command palette's "Open key bindings file"; `make keys` prints the effective bindings outside the running shell, for a bug report
- **Health check** — `oma-messenger-service doctor`, or "Run health check" in the command palette, reports the data directory's permissions, the database, the media cache, each account's connection and notify-send availability, without exposing any of their contents
- **Light on memory** — about 120 MB in use, against about 2 GB for Telegram Desktop and WhatsApp Web running side by side (see `docs/BENCHMARK.md`)
- **Install** — opening the window downloads and verifies the matching helper release the first time it is missing or out of date, with no download at plugin load time and no click needed, and **Ctrl+R** retries a failed install; installing it also adds OmaMessenger to Omarchy's apps menu

### Licensing

- Release helper binaries link go.mau.fi/libsignal (GPL-3.0) for WhatsApp's encryption, so release binaries are distributed under GPL-3.0; this repository's own source stays MIT. See the README's License section and `THIRD_PARTY_NOTICES` in each release
