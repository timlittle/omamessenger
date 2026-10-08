# Changelog

## 0.3.0 — first release

The first tagged release: a keyboard-first unified messaging client for Omarchy, with Telegram and WhatsApp connectors.

### Features

- **Telegram sign-in and sync** — add an account by QR code or phone number and code, with two-step verification support; recent chats sync on connect, and live messages, sends and read receipts stay in step with the app
- **WhatsApp sign-in and sync** — add an account by QR code or phone number and an 8-character code, linking this as a companion device; recent chats sync on connect, and live messages, sends, receipts, media (including albums), reactions, replies, pin and archive, business messages' own text and "Message yourself" all stay in step with the phone, including a message deleted on either side
- **Media** — send and receive photos, videos and files; photos open in an in-window viewer sized to the window, with a blurred preview while the full image downloads; paste an image from the clipboard straight into the composer
- **Voice notes** — play inline in the message bubble, with a scrubber and elapsed/total time; without `qt6-multimedia` installed, a voice note opens in your default player instead
- **Replies** — reply to any message, with a quote shown in the bubble that scrolls to the original when clicked
- **Reactions** — react to a message with a short emoji picker; one reaction per person, kept in step with Telegram and WhatsApp
- **Deleting a message** — delete a message yourself (**d**), for everyone or just for you; a deletion made elsewhere syncs in too
- **Search** — full-text search across every conversation, including archived ones, matching a chat's title as substring text and message bodies word by word as a prefix, ignoring case and accents
- **Pin and archive** — pin or unpin, and archive or unarchive, a conversation, kept in step with the service
- **Unread view** — **Ctrl+Shift+A** shows every unread conversation across every service and account in place of the usual list, and **Ctrl+K**'s jump-to-conversation list sorts unread conversations first
- **Keyboard navigation between conversations** — **Ctrl+J** and **Alt+↑/↓** move to the next or previous unread or any conversation, from the list, an open conversation or the unread view, keeping whichever mode — writing or scrolling — you were in
- **Hide and show all** — chats with no activity for a month fold out of the list, and you can hide a chat on this computer only; **Show all** brings both back, dimmed and labelled
- **History and link previews** — older Telegram messages load as you scroll back, and links show a preview card
- **Tables** — Markdown tables in messages render as tables
- **Writing and scrolling modes** — the composer is dimmed while you scroll and shows which keys switch between the two
- **Keyboard-centric dialogs and message actions** — adding or removing an account, and the close-window question, all work from the keyboard with a visible highlight; a highlighted message can be opened (**Enter**), replied to (**r**), reacted to (**e**), have its link opened (**o**), jump to the message it replies to (**p**), retried if it failed (**t**), or deleted (**d**)
- **Group chat senders** — each sender in a group chat gets a stable colour and an initials avatar on the first message of a run, so consecutive messages from different people are easy to tell apart
- **Notifications** — desktop notifications for new messages, with an option to include or hide the message preview; clicking a notification opens its conversation
- **Keyboard and command palette** — every action reachable from the keyboard, Slack-style shortcuts, and a command palette (**Ctrl+/**) that lists every command with its shortcut
- **Light on memory** — about 100 MB in use, against about 1 GB for Telegram Desktop and WhatsApp web running side by side
- **Install** — opening the window downloads and verifies the matching helper release the first time it is missing or out of date, with no download at plugin load time and no click needed, and **Ctrl+R** retries a failed install; installing it also adds OmaMessenger to Omarchy's apps menu

### Known limitations

- WhatsApp's older history does not load further back as you scroll; only what syncs when you link the device is available
- The helper does not yet catch up on everything that happened while it was offline; a long gap between runs may miss updates until the next full sync
- Files attached to outgoing messages are kept in `media/outgoing/` for retries but are not cleaned up once the message is safely delivered

### Licensing

- Release helper binaries link go.mau.fi/libsignal (GPL-3.0) for WhatsApp's encryption, so release binaries are distributed under GPL-3.0; this repository's own source stays MIT. See the README's License section and `THIRD_PARTY_NOTICES` in each release
