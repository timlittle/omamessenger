# Changelog

## 0.3.0 — first release

The first tagged release: a keyboard-first unified messaging client for Omarchy, with a working Telegram connector.

### Features

- **Telegram sign-in and sync** — add an account by QR code or phone number and code, with two-step verification support; recent chats sync on connect, and live messages, sends and read receipts stay in step with the app
- **Media** — send and receive photos, videos and files; photos open in an in-window viewer sized to the window, with a blurred preview while the full image downloads; paste an image from the clipboard straight into the composer
- **Replies** — reply to any message, with a quote shown in the bubble that scrolls to the original when clicked
- **Reactions** — react to a message with a short emoji picker; one reaction per person, kept in step with Telegram
- **Search** — full-text search across every conversation, including archived ones, matching a chat's title as substring text and message bodies word by word as a prefix, ignoring case and accents
- **Pin and archive** — pin or unpin, and archive or unarchive, a conversation, kept in step with the Telegram app
- **Hide and show all** — chats with no activity for a month fold out of the list, and you can hide a chat on this computer only; **Show all** brings both back, dimmed and labelled
- **History and link previews** — older messages load as you scroll back, and links show a preview card
- **Tables** — Markdown tables in messages render as tables
- **Writing and scrolling modes** — the composer is dimmed while you scroll and shows which keys switch between the two
- **Notifications** — desktop notifications for new messages, with an option to include or hide the message preview; clicking a notification opens its conversation
- **Keyboard and command palette** — every action reachable from the keyboard, Slack-style shortcuts, and a command palette (**Ctrl+/**) that lists every command with its shortcut
- **Install** — opening the window downloads and verifies the matching helper release the first time it is missing or out of date, with no download at plugin load time and no click needed; installing it also adds OmaMessenger to Omarchy's apps menu

### Known limitations

- WhatsApp is not supported yet; only Telegram accounts can be added
- The helper does not yet catch up on everything that happened while it was offline; a long gap between runs may miss updates until the next full sync
- Files attached to outgoing messages are kept in `media/outgoing/` for retries but are not cleaned up once the message is safely delivered
