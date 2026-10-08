# Build plan

What is left to build, in order. Product scope is in [FORGE_SPEC.md](../FORGE_SPEC.md), coding rules in [.claude/rules/](../.claude/rules/), and the reasons behind past choices in [decisions.md](decisions.md).

Each brief stands on its own: hand one to a contributor or an agent together with the rules. Brief IDs exist only in this file; never put them in code, comments, tests or commit messages.

**Done means:** `make check` passes, every *accept* item holds, the README and this plan are updated, and for visible changes the flow has been tried in the installed plugin (`make install-local`) with keyboard and mouse.

## UI spec

The UI briefs share this spec.

### Layout

- `FloatingWindow` titled `OmaMessenger`, implicit size `Style.space(1120)×Style.space(760)`, minimum `Style.space(760)×Style.space(540)`. Content fills the window.
- Three columns: the rail is `Style.space(64)` wide, the list is `clamp(Style.space(260), 0.32 × (width − rail), Style.space(360))`, and the conversation takes the rest. A key-hint footer `Style.space(24)` high spans the width.
- Rail entries: `all`, `service:whatsapp`, `service:telegram`, and `account:<id>` under a service only when it has more than one account. In demo mode the rail shows a `DEMO` chip with the tooltip "Seeded demo data. WhatsApp and Telegram are not connected."

### Behaviour

- State that must survive the panel being hidden lives in `service.uiState`: `railKey`, `selectedId`, `activeId`, `pane`, `query`, `drafts{conversationId: text}`.
- The selection is a conversation id, never an index, so reordering never moves it to another chat.
- Opening a conversation sends `conversations.markRead` and `ui.setFocus`, focuses the composer and restores its draft. Leaving it or hiding the window sends `ui.setFocus` with an empty `conversationId` and `windowActive: false`.
- Lists are `ListModel`s updated in place from events; never replace a whole model because of one event.
- Messages: a `ListView` with `verticalLayoutDirection: ListView.BottomToTop`, newest at index 0. Within one screen of the oldest loaded message, and while `hasMore`, request `messages.list` with `before` set to the oldest id. A new message keeps the view at the newest end if it was already there.
- `open(payloadJson)` shows the window and opens `conversationId` if the payload has one. If the window is already visible on Hyprland, focus it with `Hyprland.dispatch("focuswindow title:^OmaMessenger$")`.

### Look

- Row hover `Style.hoverFill`; selected row `Util.alpha(Color.accent, Style.selectedFillAlpha)` plus a 2 px `Color.accent` bar on the left.
- Unread: bold title and a badge (background `Color.accent`, text `Color.background`). A muted chat's badge uses `Util.alpha(Color.foreground, 0.25)`.
- Bubbles: outgoing `Util.alpha(Color.accent, 0.22)`, incoming `Util.alpha(Color.foreground, 0.06)`, text `Color.foreground`, at most 72 % of the pane wide. A failed message shows `Not sent · r to retry` in `Color.urgent`; clicking it retries.
- Status glyphs: pending `○`, sent `✓`, delivered `✓✓`, read `✓✓` in `Color.accent`, failed `!`.
- Message text: a read-only, selectable `TextEdit` with rich text from `Format.linkify(Format.escapeHtml(text))`; links open with `Qt.openUrlExternally`.

### Keys

Contexts, highest priority first: `help`, `dialog`, `search`, `compose`, `conversation`, `list`. A context binding wins over a global one. Global bindings use Ctrl, Alt or an F-key, or are Escape, so they never steal typing.

| Action | Keys | Context |
| --- | --- | --- |
| search.focus | Ctrl+K | global |
| chat.new | Ctrl+N | global |
| rail.all / rail.whatsapp / rail.telegram | Ctrl+0 / Ctrl+1 / Ctrl+2 | global |
| rail.next / rail.prev | Ctrl+Tab / Ctrl+Shift+Tab | global |
| help.toggle | F1 | global |
| demo.inject | Ctrl+Shift+D | global, demo only |
| escape | Escape | global |
| cursor.down / cursor.up | j, Down / k, Up | list |
| cursor.top / cursor.bottom | g, Home / G, End | list |
| chat.open | Enter, l, o, i | list |
| pane.conversation | Tab | list, when a conversation is open |
| scroll.down / scroll.up | j, Down / k, Up | conversation |
| scroll.pageDown / scroll.pageUp | Ctrl+D, PageDown / Ctrl+U, PageUp | conversation |
| scroll.newest / scroll.oldest | G, End / g, Home | conversation |
| compose.focus | i, a, Enter | conversation |
| chat.next / chat.prev | J / K | conversation |
| pane.list | h, Tab | conversation |
| message.retry | r | conversation |
| chat.mute | m | list, conversation |
| unread.next | u | list, conversation |
| search.focus | / | list, conversation |
| help.toggle | ? | list, conversation, help |
| window.hide | q | list, conversation |
| message.send | Enter (Shift+Enter inserts a newline) | compose |
| search.accept | Enter, Down | search |
| dialog.down / dialog.up | Down, Ctrl+J / Up, Ctrl+K | dialog |
| dialog.accept | Enter | dialog |
| dialog.nextAccount | Ctrl+Tab | dialog |
| help.close | q | help |

Escape does the first that applies: close help; close the dialog; clear a non-empty search; leave an empty search; leave the composer; close the open conversation; clear the search query; hide the window.

## UI briefs

### U1 · Walking skeleton

Status: built.

The installed plugin works on demo data, end to end.

- `ui/service/HelperProcess.qml`: runs `bin/oma-messenger-service --demo` with `Process` (`stdinEnabled: true`, stdout through `SplitParser`). Exposes `status` (`starting`, `ready`, `stopped`, `error`, `missing`) and `detail`. It restarts after 1 s, 3 s and 10 s, and gives up with `error` after five exits in 60 s. Exit status 3 from the launcher means no helper is installed: `missing`, no restart. `install()` runs `scripts/install-helper.sh` and starts the helper when it succeeds.
- `ui/service/RpcClient.qml`: `request(method, params, callback(error, result))` over any transport with `write(line)` and a `line` signal, using `ui/lib/Rpc.js`. 15 s timeout; pending requests fail when the helper restarts; notifications are emitted as `event(name, data)`.
- `ui/service/AppState.qml`: `accounts`, `unreadTotal`, `demo`, `version`, `uiState`, updated from events.
- `ui/Service.qml`: composes the three (no `required` properties) and exposes `status`, `detail`, `demo`, `unreadTotal`, `accounts`, `uiState`, `request()`, `event`, `installHelper()`, `applySettings(settings)`. Sends `hello` once ready.
- `ui/Panel.qml` (skeleton): conversation list on the left; on the right the selected conversation's messages and a text field that sends on Enter. It refreshes from events, and shows the helper status with an "Install helper" button while `missing`.
- `manifest.json`: the `service` and `panel` entry points move to `ui/`. `scripts/install-local.sh` stages `ui/` instead of the root QML files.
- **Accept:** `make install-local` opens a window listing the 11 demo conversations, and a sent message turns delivered.

### U2 · Keymap

Status: built.

- `ui/lib/Keymap.js` holds the key table above as data: `{action, keys, contexts, label, hint, demoOnly}`. `match(context, key, modifiers, text, demo)` returns the action or `""`. `bindingsFor(context)` serves the footer hints and `helpSections()` the help sheet.
- `ui/lib/Navigation.js`: `keyContext(state)` and `escapeAction(state)` as specified above.
- Replaces `keyboard.js`.
- **Accept:** a test per table row and per Escape step; `j` does nothing in compose or search; a test checks that no global binding is a plain printable key.

### U3 · List and timeline logic

Status: built.

Pure functions in `ui/lib`, one file each, with node tests:

- `Rail.js`: `items(accounts, conversations)` gives rail entries with unread sums (muted excluded) and the worst account status; `filter(conversations, railKey)`; `next(items, key, delta)` wraps.
- `Selection.js`: `move(ids, selectedId, delta)`, `edge(ids, "top"|"bottom")`, `nextUnread(conversations, selectedId)` (wraps, skips muted).
- `ListSync.js`: `planSync(oldIds, newIds)` returns minimal remove/insert/move operations; `upsertById(list, item, compare)`.
- `Timeline.js`: `annotate(newestFirst, isGroup, nowMs)` returns `{showDay, dayLabel, showSender, groupedWithOlder}` per message.
- **Accept:** `planSync` is checked on 500 random pairs (applying the operations to the old list gives the new one); the JS coverage gate holds.

### U4 · Views

Status: built.

Each view takes data through properties and reports intent through signals; none calls the helper.

- `ServiceRail.qml`: glyph, label, unread badge and status dot per entry, with tooltips; `+` (New chat · Ctrl+N) and `?` (Shortcuts · F1) at the bottom; the DEMO chip.
- `ConversationRow.qml`, `ConversationList.qml`: avatar, title (bold when unread, elided), time, preview or highlighted search match, mute icon, badge. Empty states: "No conversations yet · Ctrl+N to start one" and "No chats match “query”".
- `MessageDelegate.qml`: day separator, sender name in groups, bubble, time and status glyph, retry link.
- `ConversationView.qml`: header (title, then members, typing or account status), message list, `Composer`; empty state "Pick a chat · j/k to move · Enter to open".
- `NewChatDialog.qml`: account chooser, contact search and list; Enter opens.
- `ShortcutHelp.qml`, `KeyHints.qml`: built from `Keymap`.
- **Accept:** qmllint is clean; long titles elide at the minimum window size.

### U5 · Controllers

Status: built.

Non-visual objects in `ui/controllers/`. They are the only UI code that calls `service.request`, and they keep durable state in `service.uiState`.

- `ListController`: rail key, search, the visible conversations, selection, unread jump, mute.
- `ConversationController`: the open conversation, its messages and paging, send, retry, mark read, focus, drafts, typing.
- `DialogController`: the new-chat dialog, contacts and opening a conversation.
- `WindowController`: help, hiding, demo inject, rail switching.
- `ui/lib/Actions.js` maps each key action to its controller; a test checks that it covers exactly the actions in `Keymap`.

### U6 · Panel

Status: built.

`ui/Panel.qml` replaces the skeleton. It composes the views and controllers. A single `routeKey(event)` matches the key in the current context and runs the owning controller's action; it accepts the event only if an action ran. No helper calls or business logic. Implements `open(payloadJson)` and `close()`.

### U7 · Bar widget and settings

Status: built.

- `ui/BarWidget.qml`: icon with an unread count, dimmed unless the helper is ready; tooltip `OmaMessenger · N unread`; click toggles the window; forwards the plugin settings with `service.applySettings`.
- `manifest.json`: add the `bar-widget` kind and the settings schema (`notifications`, `notificationPreview`, `demoChatter`, all on by default).
- Delete the scaffold: `Panel.qml`, `Service.qml`, `keyboard.js` and their tests in the repository root.

### U8 · QML tests

Status: built.

`tests/qml/`: run the real Service and Panel offscreen (`QT_QPA_PLATFORM=offscreen quickshell -p <root>`) against the demo helper, driving them with real key events. Cover:

- startup with 3 accounts and 11 conversations
- open with `j j Enter`, unread clears
- send until delivered; Sam's failure, `r`, then delivered
- the Escape chain
- search for "ticket"
- Ctrl+2 rail filter
- new chat with Ben Okafor
- bar count follows the unread total
- state survives the panel being recreated
- every pane is visible at the minimum size
- the missing-helper install flow

`make check` runs them; CI runs them in an Arch container with Omarchy's shell checked out.

## Real services

Status: items 1–7 are built; item 8, the release, is in progress.

Do these after the UI. Connectors never reach real services in tests: use recorded or constructed library data with golden files, and fuzz every normalizer.

1. **Connector conformance suite:** one shared test that every connector passes. It covers connect, cancel, send progress, incoming fields, duplicate deliveries and goroutine cleanup. The demo connector passes it first.
2. **Authentication API:**
   - methods `accounts.add`, `auth.submit` (phone, code, password), `accounts.logout`, `accounts.remove`
   - events `auth.qr`, `auth.step`, `auth.done`, `auth.failed`
   - an optional `Authenticator` connector interface
3. **Account setup UI:** add an account from the rail; QR with countdown; phone, code and password steps. Built, except the QR countdown.
4. **WhatsApp connector** with whatsmeow, in four steps:
   - pairing, with its session in `<data-dir>/whatsapp/<account>.db`
   - history sync
   - live receive, receipts and typing
   - send and read receipts
5. **Telegram connector** with gotd/td, in four steps:
   - login with phone, code and 2FA, with the API id and hash from settings
   - dialog sync, loading history on demand
   - updates with gap recovery, including the gap while the helper itself was not running: gotd's update-manager state persists per account, so a restart resumes updates.getDifference from where it left off
   - send and read
6. **Richer messages:** edits and deletes, replies, reactions, media in and out. Built, with voice notes, and deleting from OmaMessenger as well.
7. **Finishing:**
   - full-text search (SQLite FTS5)
   - pin and archive
   - clicking a notification opens its chat
   - demo off once a real account exists (the demo mode was removed instead; fake accounts exist only in test builds)
8. **Release:**
   - document the privacy and account risks in the README
   - bump the version and tag (version bumped to 0.3.0; not tagged yet)
   - confirm `install-helper.sh` installs it on a clean machine

## After 0.3.0

Ideas from comparing other Omarchy messaging plugins, not yet scheduled.

- Reply from the desktop notification without opening the window, as omarchy-signal does.
- Send later: a message queued in the helper and sent at a chosen time while the helper runs, as Beeper offers.

## Later, only if it becomes a problem

Not planned. Build one of these only when the problem it solves is actually seen.

Helper resilience:

- Never give up restarting a crashing helper: after the fast retries, keep trying every minute, and at once when the window opens.
- Hold requests while the helper restarts and send them when it is back; show an error only if it stays down for more than about 30 seconds.
- Retry a failed send automatically once after a reconnect before marking it "Not sent". (Done: the helper retries automatically on reconnect and on a backoff - 30s, 2m, 10m, 1h, then hourly - for up to 24h, classifying a service's refusal as permanent or worth retrying; see docs/decisions.md.)
- One helper at a time: a lock file in the data directory, so a replacement waits for the old helper to exit instead of sharing its Telegram session.
- Show "Reconnecting…" on the account in the rail instead of an error line across the window.
