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

Status: built, including the release (item 8).

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
8. **Release:** done. The privacy and account risks are documented in the README; release-please bumps the version and tags each release (see CONTRIBUTING.md's "Release" section for the checksum-pinning follow-up pull request each one needs); `install-helper.sh` installs a tagged release on a clean machine.

## Signal

Status: planned. Signal joins as a third service on the same footing as WhatsApp and Telegram, through `go.mau.fi/mautrix-signal`'s `signalmeow` inside the helper (see "Signal: built on signalmeow" in decisions.md). Do the briefs in order; each ends with `make check` and `make ci` passing.

Signal itself sets two limits no brief can lift: a linked device gets its history once, at link time, and only if the user picks "Transfer message history" on the phone ("Don't transfer" means no history at all); media older than 45 days is gone from Signal's servers.

Already proven against `signalmeow` v26.10 (libsignal v0.105.0): the dependency resolves next to the pinned whatsmeow; libsignal builds for x86_64 and arm64; the x86_64 helper links and the whole Go suite passes with cgo and `-race`; `signalmeow`'s store migrates cleanly on `modernc.org/sqlite`; nothing in `signalmeow` writes to stdout. Not yet proven: the arm64 helper link (a cross container lacked arm64 zlib; native arm runners avoid that) and a real link with history transfer, which needs a phone.

### S1 · cgo build and AGPL release

The helper now needs cgo and Signal's Rust library, `libsignal_ffi`, at the exact version `signalmeow` pins (`pkg/libsignalgo/signalversion`).

- `make libsignal` fetches signalapp/libsignal pinned to a full 40-character commit, checked out detached and verified against that commit (a tag alone is a marketplace finding, `remote-git-execution-unpinned`). It builds `libsignal-ffi` once into `build/libsignal/<version>/<arch>/`, with `CARGO_TARGET_DIR` under `build/` and `CARGO_BUILD_JOBS` capped (about 3.5 GB peak), and skips the build when the archive exists. Needs rustup (it honours libsignal's `rust-toolchain`), cmake, protoc and clang; document them in CONTRIBUTING.
- cgo is on by default wherever a C compiler exists (`go test -race` already needs it), so `//go:build cgo` does not hide the Signal connector from tools: it makes them fail to link. Export `CGO_ENABLED=1` and `CGO_LDFLAGS=-L<archive dir>` once in the Makefile and make `libsignal` a prerequisite of every Go target: `build`, `test-build`, `test-go`, `lint` (golangci-lint and `tools/nologcontent`), `license-check`, `third-party-notices`, `vulncheck`. `build-fake` keeps `CGO_ENABLED=0`.
- Providers: `backend/signal_cgo.go` (`//go:build cgo`) returns the Signal provider and `backend/signal_nocgo.go` (`//go:build !cgo`) returns none, mirroring `fake.go`/`nofake.go`; `providers()` in `main.go` appends them. Add the depguard rule for the new package.
- CI: `ci.yml`'s jobs build libsignal through `actions/cache` keyed on the commit and architecture. The `release-build` dry run becomes a two-runner matrix so arm64 is still built before merge. Rust comes from the runner's rustup or a SHA-pinned action, never `curl | sh`. CodeQL builds with cgo too, so the Signal connector is scanned.
- Release: `release.yml`'s single job becomes a matrix on `ubuntu-24.04` and `ubuntu-24.04-arm` (glibc 2.39; the 22.04 images retire on 17 April 2027); a final job gathers both binaries for checksums and attestations. `scripts/build-release.sh` builds only the host architecture. `release-check` fails if `ldd` shows anything beyond libc, libm, libz, libstdc++, libgcc_s, libresolv and libdl.
- `install-helper.sh`: raise the download cap from 64 MiB to 128 MiB; the helper grows from 31 MB to about 54 MB.
- Licence: release binaries become AGPL-3.0. Add `LICENSE-AGPL-3.0`, update the README's License section and decisions.md, add `AGPL-3.0` to `ALLOWED_LICENSES`. go-licenses cannot see Rust crates, so `THIRD_PARTY_NOTICES` also takes libsignal's own generated acknowledgments (`acknowledgments/` at the pinned commit) through `scripts/third-party-notices.tmpl`. The repository source stays MIT.
- Accept: `make check` and `make ci` pass on a clean checkout; the release dry run produces both binaries, each runs `--version` on its own architecture, and the `ldd` check passes.

### S2 · Disappearing and view-once messages, every service

Today a disappearing message from WhatsApp or Telegram is stored forever (WhatsApp's normalizer unwraps the ephemeral and view-once wrappers and keeps the content as an ordinary message), and a Signal one would be too. Senders turn these on expecting the message to go. This brief makes the helper honour them for all three services, before Signal arrives.

- Domain and store: a message carries its expiry period and when its timer starts; the store keeps the resulting `expires_at` and re-arms from it after a restart. A sweeper goroutine in `app`, shaped like the retry scheduler, deletes each expired message's text, media files and search entry at its time and emits the same event a delete does. It never sends a delete to the service.
- Timer start follows each service's own rule, reported by its connector: Signal starts a received message's timer when it is read and a sent one's when it is sent ([Signal support](https://support.signal.org/hc/en-us/articles/360007320771-Set-and-manage-disappearing-messages)); WhatsApp counts from sending, read or not ([WhatsApp FAQ](https://faq.whatsapp.com/general/chats/about-disappearing-messages/)); Telegram's message `TTLPeriod` rule is checked against its documentation before coding. Messages already past their expiry are never imported from history.
- Connectors: WhatsApp reads `ContextInfo.Expiration`, Telegram `TTLPeriod`, Signal `DataMessage.ExpireTimer`, and each reports timer changes in a chat.
- View-once media is never downloaded: the message shows "View-once photo: open it on your phone" (or video, or voice note). Signal's `IsViewOnce`, WhatsApp's view-once wrappers and Telegram's self-destructing media all map to it.
- Protocol: an optional `expiresAt` on a message and a `viewOnce` flag, added without breaking older UIs; `docs/api.md` updated. The UI shows a timer glyph with a tooltip giving the time left.
- Accept: in synctest, a received Signal message expires its period after being read and a WhatsApp one its period after being sent, with text, media file and search entry gone and the UI told; a restart keeps pending expiries; a view-once photo is never fetched.

### S3 · Signal connector: link and receive

`backend/internal/connector/signal`, shaped like the WhatsApp connector: the real `signalmeow` client sits behind a small interface with a hand-written fake, as WhatsApp's `device` does, so the 90 % coverage gate holds without exclusions.

- `domain.ServiceSignal`, and `domain.ValidService` accepts it; without that, `store.UpsertAccount` rejects the account.
- `Provider` and `Authenticator`: link as a secondary device by QR (`PerformProvisioning` with history allowed), with the same countdown and steps as WhatsApp. Session in `<data-dir>/signal/<account>.db` (0600, directory 0700), opened with `dbutil.NewWithDB(db, "sqlite3")` over our `modernc.org/sqlite` connection.
- Logging: `main.go` calls `signalmeow.SetLogger` once at start-up (it is process-wide and unsynchronised), keeping libsignal's own logs at Warning and above on stderr. Each client gets a no-op zerolog logger, as whatsmeow gets `waLog.Noop`: `signalmeow` logs the whole backup info, including the media backup key, at Info level (`backup.go:185`).
- History: after link, `WaitForTransfer` then `FetchAndProcessTransfer` import the archive into `signalmeow`'s backup store. Import each chat with its archived, pinned-order and mute-until state and its newest page of messages. Implement `HistoryLoader` by paging older messages out of that local archive (`GetBackupChatItems` takes an anchor and a limit), so scrolling back works as it does for Telegram, up to the start of the transferred history.
- Live receive through the client's event handler: 1:1 and groups v2, contacts, names and avatars, receipts, typing, edits, deletes, reactions, polls (create, vote, end), stickers, voice notes, disappearing timers and view-once (handled as the disappearing-messages brief sets out). Reactions, deletes, polls and timers arrive as fields on the raw `DataMessage`; the normalizer reads each one.
- Identity: a message's remote ID is its sender ACI plus sent timestamp, which reactions, edits, deletes and votes target. A contact first seen by phone number or PNI moves to its ACI through `RemoteIDUpgrader`, as Telegram's placeholder peers do.
- Keep per message what later actions need: the attachment pointer fields (CDN id, key and number, key, digest, size) for `MediaFetcher`, and the server GUID and report-spam token of incoming messages (an edit carries neither, so keep the original's).
- Safety: run every link-preview and formatting-span URL through `domain.AllowedLinkURL`, and every inline thumbnail, avatar or sticker preview through `imagesafe`, as the other connectors do; fetched media is already checked centrally in `app/media.go`.
- `MemberLister`; `LogoutOnRemove` through `Unlink`, which removes the device from the account.
- Tests: the conformance suite, plus golden files built from constructed `signalpb` messages and backup frames, never from a real account. Fuzz every normalizer.
- Accept: a test account links, its history appears and pages back, new messages arrive live, and nothing listed in privacy.md reaches the logs.

### S4 · Signal connector: send and act

- Send text with replies, mentions and formatting, media and voice notes, through `SendMessage`/`SendGroupMessage` with `WrapDataMessage`; `UploadAttachment` for files.
- `Reactor`, `MessageEditor` (`WrapEditMessage`), `Deleter` (delete for everyone within Signal's own time limit), `Voter`: Signal votes by option index plus a per-voter vote counter, which the connector stores, since `signalmeow` keeps none.
- `MarkRead`: read receipts go to each sender's ACI (`ReadReceptMessageForTimestamps`); `signalmeow` also sends the read sync to the user's own devices, so the phone clears its unread count.
- `Blocker`: a message-request-response sync message to the user's own devices (1:1 only; `app/block.go` already refuses groups). `SpamReporter`: `ReportSpam` with the stored server GUID and token of that sender's recent messages, then the block-and-spam sync. Both are a few lines over exported `signalmeow` calls, as its bridge does them.
- `AttachmentLimiter`: 100 MiB.
- Accept: each action shows on the phone; the retry and refusal classes behave as for the other two services.

### S5 · Pin, archive and mute stay on this computer

`signalmeow` can read Signal's synced chat settings but cannot write them. So the Signal connector does not implement `Organizer`, and `connector.Manager` keeps pin and archive local, as it does for any connector without one; mute is already local for every service. A Signal chat starts from the state in the transferred history; later changes on the phone do not arrive (`signalmeow` raises no event for them), and changes here do not reach the phone. The README says so in one line. A later brief can poll `FetchStorage` for phone-side changes and, separately, write them back through the storage service.

### S6 · Signal in the UI

- Nerd Fonts has no Signal logo, so `ServiceGlyph.qml` uses `md-message_lock` (U+F0FCC, present in the installed JetBrainsMono Nerd Font) with the label "Signal".
- `Rail.js` and `Tooltip.js` service lists; `Keymap.js` `rail.signal` on Ctrl+3 (free) and `setup.chooseSignal` on `s` (free in that dialog); the matching entries in `Actions.js`, `AccountController.qml` and `ListController.qml`; `Setup.js` mnemonic `s`.
- Notification sound: a `signalNotificationSound` setting in `Settings.js` and `manifest.json`, and `Service.qml`'s two-way ternary (`data.service === "telegram" ? … : whatsappSound`, which would give Signal WhatsApp's sound) becomes a lookup by service. The helper side is already generic.
- `AccountSetup` handles the QR flow as for WhatsApp; its test mocks the RPC layer, so no fake Signal provider is needed. Add a Signal account to the fake scripts so the rail and flow tests cover it.
- Docs: README (setup, account risk, the S5 limit, "Don't transfer" means no history), `docs/shortcuts.md` (Ctrl+3, `s`), the example payloads in `docs/api.md`, the manual checks doc.
- Accept: the offscreen QML tests cover a Signal account in the rail, its glyph and sound, and the setup flow; the flow has been tried in the installed plugin with a test account.

## macOS

Status: planned, after the terminal client. The plugin lives inside Omarchy's shell and stays Linux-only, so on a Mac the terminal client is the only front end and the helper is what gets ported. The service connectors, the store and the JSON-RPC API are portable Go already; libsignal builds for macOS. Do the briefs in order; each ends with `make check` and `make ci` passing, and a run on a macOS CI runner once M1 exists.

### M1 · Build and test on macOS

- Split the Linux-only code behind build tags (`_linux.go` / `_darwin.go`), starting with `backend/stdout.go` (`Dup3`), the sandbox, notifications, the portal, the clipboard and the doctor. A darwin build of each starts as "not available", so the helper links and runs with those features off.
- Data under `~/Library/Application Support/omamessenger` (0700, files 0600), not the XDG directories; `backend/config.go` picks by OS.
- CI: a `macos-latest` job (Apple Silicon) builds libsignal and runs `go test -race` on the portable packages, with the same coverage gates. Linux jobs unchanged.
- Accept: the helper builds on macOS, `--version` runs, and the connector, store, app and server tests pass on the macOS runner.

### M2 · Media sandbox on macOS

The hardest brief, and the one that decides whether video and voice notes play in the client at all. On Linux, ffmpeg runs inside bubblewrap with a seccomp filter and prlimit caps (see decisions.md); none of these exist on macOS.

- Evaluate macOS's own sandbox (`sandbox-exec` with a Seatbelt profile: no network, read only the input, write only the one output file) plus `setrlimit` caps, against the security review's threat model. Write the comparison in decisions.md before any code: what each Linux protection maps to, and what has no equivalent.
- If it holds: a `sandbox_darwin.go` with the same `Run` contract as Linux, and the transcode tests run on the macOS runner. If it does not: media on macOS opens in the system player only, and `playable` stays false there.
- Accept: the decision recorded; either the sandboxed transcode passes the existing transcode tests on macOS, or `media.fetch` reports `playable:false` on macOS with a test.

### M3 · Desktop integration

- Opening links and files: `open` instead of the XDG portal, behind the same allow-lists `openexternal` uses on Linux.
- Clipboard: `pbcopy` instead of `wl-copy`.
- Notifications: `osascript` (`display notification`) first; native notifications need a signed app bundle, which waits for M5.
- Local socket for the terminal client: `getpeereid` instead of `SO_PEERCRED`, and a per-user socket directory, since macOS has no `$XDG_RUNTIME_DIR`.
- Doctor: report the macOS tools instead of the Linux ones.
- Accept: each has a unit test through its existing seam; the manual check list covers a link, a copy and a notification on a real Mac.

### M4 · Release for macOS

- `release.yml` gains `macos-latest` (arm64) and an Intel runner while GitHub offers one; libsignal built natively on each. The `ldd` check gets an `otool -L` counterpart allowing only system libraries.
- Notices and AGPL licence as for Linux.
- Accept: the release dry run produces both macOS binaries, each runs `--version` on its own architecture.

### M5 · Signing and install

- Code signing and notarization, so Gatekeeper accepts the binaries without a warning. Needs an Apple Developer account (a paid yearly membership) and its signing secrets in the release workflow.
- An install path for macOS: a Homebrew tap, or the terminal client's own verified helper install (the same checksum and version pinning as `install-helper.sh`).
- Accept: a fresh Mac installs the client and helper, links an account and sends a message, with no Gatekeeper prompt.

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
