# OmaMessenger implementation tasks

Spec-driven task list. `FORGE_SPEC.md` is the product contract; this file is the build plan.
Contracts (§2) are normative: tasks reference them instead of restating them.

## 0. Rules for the implementer

1. Do tasks in ID order unless `deps` allow otherwise. Never start a task whose deps are unchecked.
2. Touch only the files listed in `files`. If another file must change, stop and add a note under the task.
3. A task is done only when every `accept` item holds and the `verify` command exits 0. Then change `[ ]` to `[x]`.
4. Do not edit Omarchy's packaged source (`/usr/share/omarchy`). Do not use `sudo`. Do not drive the developer's live Hyprland session from tests.
5. Never log message text, contact names, credentials, QR data or session keys (stderr included).
6. Go: `gofmt`, `go vet` clean; build with `-mod=vendor` (see F9). QML JS libraries: see C8.
7. Status markers: `[ ]` todo, `[x]` done, `[~]` partially done (the note says what remains).
8. Every phase ends with a gate task (`GA`, `GR`, `GB`, `GQ`, `GD`). No task in a later phase may start until the previous gate is `[x]`. A gate is checked only when every item in §0.1 and the gate's own list passes. A gate may scope a check to the code that exists at that phase and name the later gate that will enforce its full form. If a check fails, fix the cause in the owning task (re-open it as `[~]`); never waive a check permanently.
9. Respect C10. Its rules are enforced **in code**: Go analyzers (`tools/omalint`), architecture tests (`backend/internal/archtest`), a tested UI linter (`tools/uilint`) and docs checks (`tools/docscheck`), not ad-hoc shell scripts. A change that needs a new import edge, a higher limit or a suppression must first update C10 and the rule table in code, with the reason.
10. Suppressions are rare and explicit: `//omalint:ignore <rule> <reason>` (Go) or `// uilint-ignore <rule> <reason>` (QML/JS) on the line above. The reason is mandatory, and there are at most 5 suppressions per language, both enforced in code.

### 0.1 Definition of Done (every gate)

- D1 All tasks in the phase are `[x]`; none is `[~]`.
- D2 `gofmt -l backend tools` is empty; `go vet -mod=vendor ./...` is clean.
- D3 `go test -mod=vendor -race -count=3 ./backend/... ./tools/...` passes (3 runs catch flaky timing).
- D4 Coverage gates pass: `go run -mod=vendor ./tools/covergate build/cover.out` and the JS coverage flags in C9.
- D5 Rules in code pass with zero findings: `go run -mod=vendor ./tools/omalint ./backend/... ./tools/...`, `go test -mod=vendor ./backend/internal/archtest/`, `npm --prefix tools/uilint run lint`. The tools' own tests pass too (they run under `go test ./...` and `npm --prefix tools/uilint test`).
- D6 Every behaviour change has a test at the narrowest layer (CONTRIBUTING test map); the total test count never drops from the previous gate. Record it in the gate note: `go test -mod=vendor -json ./backend/... | grep -c '"Action":"pass","Package":"[^"]*","Test"'` plus `node --test` totals.
- D7 Fuzz targets each run 30 s without failure: `go test -mod=vendor -run=^$ -fuzz=<Target> -fuzztime=30s <pkg>`, once per target.
- D8 Clean-checkout build: `git stash -u` is forbidden. Instead run `git worktree add ../oma-gate HEAD` after committing, then run the phase's test command inside it, then `git worktree remove ../oma-gate`. This catches files that are only present untracked.
- D9 `git grep -nE 'TODO|FIXME|XXX' -- backend ui scripts tests` lists only lines that name a task ID (e.g. `TODO(D14)`).
- D10 Docs checks in code pass: `make docs-check` (R08, B15). It catches generated docs that are stale, contracts that disagree with the code tables, broken links, paths that don't exist, unknown `make` targets and unknown flags.
- D11 Docs reviewed and up to date. Add this table to the gate note, one row per document in the inventory below: `| doc | reviewed at <commit sha> | result: updated / no change needed | what changed |`. The reviewer reads each document in full against the code and confirms that every statement about behaviour, commands, paths, shortcuts, settings, data locations and project status is true. In particular:
  - The FORGE_SPEC.md `Status` and `Current gap` and the README `Current status` describe only what works now (no connector claims before GD).
  - CONTRIBUTING's test map names the current test layers and commands.
  - AGENTS.md layout matches C1.
  - This file's §1 facts are still true and its §2 contracts match the code.

  Inventory: README.md, CONTRIBUTING.md, AGENTS.md, FORGE_SPEC.md, .agents/README.md, docs/TASKS.md (§0–§2), docs/KEYS.md, docs/PROTOCOL.md, docs/ARCHITECTURE.md. Files that don't exist yet are marked `n/a`.

Task format:
```
### [ ] ID · Title  (size S|M|L)
- deps: IDs
- files: paths (create/modify/delete)
- do: numbered steps
- accept: checkable outcomes
- verify: command
```

## 1. Environment facts (verified 2026-10-05, Omarchy 4.0.4, Quickshell 0.3.1, Qt 6.11)

- F1 `/usr/bin/qmllint` and `/usr/bin/qmltestrunner` are **Qt 5**. Use `/usr/lib/qt6/bin/qmllint` and `/usr/lib/qt6/bin/qmltestrunner`.
- F2 `qs.Commons` imports Quickshell, whose types live inside the `quickshell` binary, so `qmltestrunner` cannot load our UI. Test QML by running `QT_QPA_PLATFORM=offscreen quickshell -p <root>`. `<root>` must contain `Commons` and `Ui` symlinks to `$OMARCHY_PATH/shell/{Commons,Ui}` (that's how `qs.*` imports resolve). `Qt.exit(code)` sets the process exit code. `import QtTest` + `TestCase { id: t; when: false }` provides `t.keyClick(...)`, which delivers real key events to the focused item.
- F3 Quickshell 0.3.1 `FloatingWindow` has **no `requestActivate()`**. Calling it throws and aborts the rest of the function.
- F4 Omarchy only keeps a panel loaded while it's open. `hide` destroys the Panel, so durable UI state must live in `Service.qml`.
- F5 Panel injection: Omarchy sets `property var service` (our Service.qml instance) and `property var shell` (a facade with `serviceFor(ownId)`, `hide(id)`, `toggle(id, json)`, `summon(id, json)`). `summon` calls `panel.open(payloadJson)` every time, including when already open. `hide` calls `panel.close()`.
- F6 Bar widget: get the service with `bar.shell.serviceFor("io.github.omamessenger")` and open the window with `bar.shell.toggle("io.github.omamessenger", "{}")`. Plugin settings arrive on the widget's `settings` property, not on the service; the widget forwards them with `service.applySettings(settings)`.
- F7 `Service.qml` must declare **no `required` properties**; the plugin fails to load otherwise.
- F8 Omarchy hot-reloads plugin code on any file write under `~/.config/omarchy/plugins/`. Keep databases and runtime files outside the plugin dir.
- F9 `vendor/` is tracked. Deleting it needs the owner's approval, so keep building with `-mod=vendor`. Adding a Go dependency requires `go mod vendor` (or owner approval to drop vendoring). R01a adds `golang.org/x/tools` this way (network needed once).
- F10 Quickshell `Process`: set `stdinEnabled: true` and use `write(string)` to send; parse stdout with `stdout: SplitParser { onRead: function(line) {...} }`.
- F11 Resolve the plugin's own files from QML with `Qt.resolvedUrl("../bin/oma-messenger-service")` and strip the `file://` prefix. This works for both installed copies and symlinked dev checkouts.
- F12 Omarchy `Style` tokens: `Style.space(px)`, `Style.font.{caption,bodySmall,body,subtitle,title,heading,display}`, `Style.spacing.*`, `Style.hoverFill`, `Style.selectedFill`, `Style.selectedFillAlpha`, `Style.cornerRadius`. `Color.{foreground,background,accent,urgent,muted}`, `Color.popups.{background,text,border}`. Helper: `Util.alpha(color, a)`. Omarchy `qs.Ui` controls: `Button`, `TextField`, `Dropdown`, `Toggle`, `BarWidget`, `BarIconButton`, `Panel`, `BorderSurface`.

## 2. Contracts

### C1 · Target layout
```
manifest.json                 kinds [service, panel, bar-widget]; entryPoints → ui/Service.qml, ui/Panel.qml, ui/BarWidget.qml
ui/Service.qml                thin entry point composing ui/service/* and exposing their API
ui/service/HelperProcess.qml  start/stop/restart the helper; status, detail; emits line(string)
ui/service/RpcClient.qml      request(method, params, cb), pending map, timeouts; emits event(name, data)
ui/service/AppState.qml       accounts, unreadTotal, demo, uiState (survives panel unload)
ui/Panel.qml                  window, pane composition, key routing to controllers; no RPC calls
ui/controllers/*.qml          non-visual; own RPC calls and action handling per area (B15a)
ui/BarWidget.qml              unread badge, click toggles window, forwards settings
ui/components/*.qml           views: take properties, emit signals, never call RPC
ui/lib/{Rpc,Keymap,Actions,Format,Rail,Selection,ListSync,Timeline,Navigation}.js   `.pragma library`, pure logic, node-tested
backend/main.go               composition root only: flags, construction, wiring
backend/internal/domain       shared types, status rules, ErrNotFound
backend/internal/store        SQLite repository
backend/internal/connector    Connector/Sink/Clock interfaces + optional sink interfaces, Manager
backend/internal/connector/demo   seeded scripted connector
backend/internal/notify       desktop notifications
backend/internal/app          Commands (UI use-cases) and Ingest (connector Sink); consumer-side ports in ports.go
backend/internal/app/policy   pure notification policy
backend/internal/rpc          generic JSON-lines server; knows nothing of app or store
backend/internal/api          adapter: method table + error→code mapping (app ↔ rpc)
bin/oma-messenger-service     launcher; prefers bin/dev/oma-messenger-service when executable
bin/dev/                      gitignored local builds
tests/unit/                   node:test files + load.cjs
tests/qml/                    quickshell offscreen harness
tests/e2e/                    Docker headless Hyprland
docs/KEYS.md                  generated from ui/lib/Keymap.js (also mirrored into README between markers)
docs/PROTOCOL.md              generated from the api method/event descriptors
docs/ARCHITECTURE.md          generated by archtest: dependency graph + coupling/cohesion metrics
tools/omalint/                Go analyzers (go/analysis) enforcing C10; rule tables in tools/omalint/rules/rules.go
tools/covergate/              per-package coverage gate (thresholds table in code)
tools/docscheck/              docs-vs-code checks and generators
tools/uilint/                 ESLint config + QML rules + keys doc generator (node, pinned dev deps)
backend/internal/archtest/    test-only package: coupling and cohesion tests over the real module
```

### C2 · Storage
- Data dir: `${XDG_DATA_HOME:-$HOME/.local/share}/omamessenger/` (mode 0700). DB file: `demo.db` with `--demo`, otherwise `messages.db` (mode 0600).
- Schema: migration 1 in `backend/internal/store/store.go`. Timestamps are int64 Unix ms. Ordering is `created DESC, rowid DESC`. Messages are unique on `(conversation_id, remote_id)` when `remote_id != ''`.
- Migrations: append to the `migrations` slice; never edit a released entry. `PRAGMA user_version` tracks progress.
- The old scaffold DB at `~/.config/omamessenger/messages.db` and `api.token` are not migrated. README tells users they may delete them.

### C3 · Helper ↔ UI protocol (stdio JSON lines, `protocol: 1`)
- Framing: one UTF-8 JSON object per line in each direction; max line 1 MiB. stdin carries requests; stdout carries responses and events; stderr carries logs only. The helper exits 0 on stdin EOF or SIGTERM.
- Request `{"id":<int>,"method":"<name>","params":{...}}`. Response `{"id":<int>,"result":<any>}` or `{"id":<int>,"error":{"code":"bad_request|not_found|unknown_method|internal","message":"..."}}`. Event `{"event":"<name>","data":{...}}`.
- Requests may be handled concurrently; stdout writes are serialized. Shapes use the JSON tags in `domain.go`.

| method | params | result | errors |
|---|---|---|---|
| `hello` | `{}` | `{protocol:1, version, demo:bool, unreadTotal}` | |
| `accounts.list` | `{}` | `Account[]` | |
| `conversations.list` | `{query?}` | `Conversation[]` (`match` set when query non-empty) | |
| `messages.list` | `{conversationId, before?:messageId, limit?:1..200 (default 50)}` | `{messages: Message[] oldest-first, hasMore}` | not_found |
| `messages.send` | `{conversationId, text}` | `Message` (status `pending`) | bad_request (empty / >4096 runes), not_found |
| `messages.retry` | `{messageId}` | `Message` (status `pending`) | bad_request if not an outgoing `failed` message |
| `conversations.markRead` | `{conversationId}` | `{}` | not_found |
| `conversations.setMuted` | `{conversationId, muted:bool}` | `Conversation` | not_found |
| `conversations.open` | `{accountId, contactId}` | `Conversation` (existing or new `direct`) | not_found |
| `contacts.list` | `{accountId, query?}` | `Contact[]` | |
| `ui.setFocus` | `{conversationId:"" or id, windowActive:bool}` | `{}` | |
| `settings.apply` | `{notifications:bool, notificationPreview:bool, demoChatter:bool}` | `{}` | |
| `demo.inject` | `{conversationId?}` | `Message` | unknown_method unless `--demo` |

| event | data | emitted when |
|---|---|---|
| `account.updated` | `{account}` | status/detail changes |
| `conversation.updated` | `{conversation}` | created, preview/unread/muted/title change |
| `message.added` | `{message}` | any new stored message (incoming or outgoing) |
| `message.updated` | `{message}` | status change that `domain.StatusAdvances` allows |
| `typing` | `{conversationId, name, active}` | connector typing signal |
| `unread.changed` | `{total}` | `store.UnreadTotal()` value changes |

Notification policy (in `app`): an incoming message notifies unless settings `notifications=false`, OR the conversation is muted, OR (`windowActive` AND conversation == focused conversation). In the last case the app also marks the conversation read. Notification title: `SenderName` for direct chats, `SenderName · Title` for groups. Body: the text if `notificationPreview` is set, else `"New message"`.

### C4 · Connector interface (`backend/internal/connector`)
```go
type Clock interface { Now() time.Time; AfterFunc(d time.Duration, f func()) (stop func() bool) }
type Sink interface {
    AccountStatus(accountID, status, detail string)
    Contact(c domain.Contact)
    Conversation(c domain.Conversation)                         // keyed by AccountID+RemoteID; ID ignored
    Incoming(accountID, conversationRemoteID string, m domain.Message) // RemoteID, SenderID, SenderName, Text, Created set
    OutgoingStatus(localMessageID, remoteID, status string)
    Typing(accountID, conversationRemoteID, name string, active bool)
}
type Connector interface {
    Account() domain.Account
    Run(ctx context.Context, sink Sink) error                   // blocks until ctx done or fatal error
    Send(ctx context.Context, conv domain.Conversation, m domain.Message) error // returns fast; progress via sink.OutgoingStatus
    MarkRead(ctx context.Context, conv domain.Conversation) error
}
```
Interface growth rule (open/closed, interface segregation): **never add methods to `Sink` or `Connector`.** A new capability is a new small optional interface in `connector.go`. Producers type-assert before use. Existing optional interfaces:
```go
type HistorySink interface { History(accountID, conversationRemoteID string, m domain.Message) } // backfill: no unread, no notify
// Added by later tasks (do not create early):
type EditSink     interface { Edited(accountID, convRemoteID, remoteID, text string, at int64); Deleted(accountID, convRemoteID, remoteID string) } // D11
type ReactionSink interface { Reaction(accountID, convRemoteID, remoteID, senderID, emoji string, active bool) }                          // D13
type HistoryLoader interface { LoadHistory(ctx context.Context, conv domain.Conversation, beforeRemoteID string, limit int) error }       // D08, implemented by connectors
type Authenticator interface { BeginAuth(ctx context.Context, sink AuthSink) error; Submit(ctx context.Context, step, value string) error } // D01
```
The Manager depends on `type AccountStore interface { UpsertAccount(domain.Account) error; SetAccountStatus(id, status, detail string) (domain.Account, error) }`, declared in `connector`. It never imports `store`.

Manager: target after R03, it takes the consumer-side `AccountStore` interface and upserts each `Account()`, runs each connector in its own goroutine, and restarts it when `Run` returns an error. Backoff is 1s, 2s, 5s, 15s, then 60s capped, reset after 5 minutes connected. Before each wait it sets status `error` with `detail = err.Error()`. `Send`/`MarkRead` route by `AccountID`. Phase A currently uses `*store.Store`; R03 removes that dependency.

### C5 · Demo connector (`--demo`)
Accounts: `wa-personal` (whatsapp, "Personal"), `tg-personal` (telegram, "Personal"), `tg-work` (telegram, "Work").

| account | title | kind/members | seeded content | unread | muted |
|---|---|---|---|---|---|
| wa-personal | Mum | direct | 14 msgs over 6 days | 0 | |
| wa-personal | Climbing Crew | group/5 (Priya, Tom, Jess, Omar) | 30 msgs, alternating senders | 3 | |
| wa-personal | Alex Chen | direct | 6 msgs; last incoming has `https://example.com/tickets` | 1 | |
| wa-personal | Sam (spotty signal) | direct | 4 msgs | 0 | |
| wa-personal | Flat 4B | group/4 | 12 msgs | 5 | yes |
| wa-personal | Dr. Bartholomew Featherstonehaugh-Wainwright (Dentist) | direct | 2 msgs | 0 | |
| tg-personal | Nadia | direct | 10 msgs incl. one 3-line msg and `مرحبا، كيف حالك؟` | 2 | |
| tg-personal | Omarchy Users | group/2400 | 150 msgs (pagination) | 12 | |
| tg-personal | Saved Messages | direct | 5 outgoing notes | 0 | |
| tg-work | Platform Team | group/9 | 20 msgs | 4 | |
| tg-work | Jordan (Manager) | direct | 8 msgs; last is outgoing with status `read` | 0 | |

Extra contacts with no conversation: wa-personal: Ben Okafor, Carla Ruiz, Dev Patel; tg-personal: Elif Kaya, Femi Adeyemi, Greta Lind; tg-work: Hana Sato, Ivan Petrov, Jo Park.

- Seed messages use RemoteID `seed-<convRemoteID>-<n>`, so reseeding is idempotent (store dedupe). Timestamps are `now - offset`. Outgoing seeded messages are `read` (DM) or `delivered` (group). Seeded unread counts come from the incoming messages left unread.
- Run: status `connecting`, then `connected` after 600 ms (`tg-work`: 2000 ms).
- Send: `sent` @250 ms, `delivered` @900 ms, `read` @2500 ms (direct only). For direct chats: typing on @3000 ms, then typing off plus an incoming reply @4500 ms, cycling through each contact's reply list.
- Failure: in "Sam (spotty signal)", the first attempt of each message goes `failed` @800 ms; a retry follows the normal path.
- Chatter (when on): every 30–90 s (uniform), one incoming message to a random conversation from its script pool. Disabled by `--no-chatter` or `settings.apply{demoChatter:false}`.
- `--seed N` fixes `math/rand`; tests inject a fake Clock.

### C6 · Keymap (`ui/lib/Keymap.js`, single source; `docs/KEYS.md` is generated from it)
Context precedence: `help` > `dialog` > `search` > `compose` > `conversation` > `list`.
Key spec syntax:
- Lowercase letter = no Shift; uppercase letter = with Shift.
- A single punctuation char (`/`, `?`) matches on event text, with Ctrl/Alt not held.
- Named keys: `Enter` (Return or Enter), `Escape`, `Tab`, `Up`, `Down`, `Home`, `End`, `PageUp`, `PageDown`, `F1`.
- Modifiers are written `Ctrl+`, `Shift+`, `Alt+`. `Shift+Tab` and Backtab are the same key.

| action | keys | contexts |
|---|---|---|
| search.focus | Ctrl+K | global |
| chat.new | Ctrl+N | global |
| rail.all / rail.whatsapp / rail.telegram | Ctrl+0 / Ctrl+1 / Ctrl+2 | global |
| rail.next / rail.prev | Ctrl+Tab / Ctrl+Shift+Tab | global |
| help.toggle | F1 | global |
| demo.inject | Ctrl+Shift+D | global (demo only) |
| escape | Escape | global |
| cursor.down / cursor.up | j, Down / k, Up | list |
| cursor.top / cursor.bottom | g, Home / G, End | list |
| chat.open | Enter, l, o, i | list |
| pane.conversation | Tab | list (only when a conversation is open) |
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
| message.send | Enter | compose (Shift+Enter is unbound, so it inserts a newline) |
| search.accept | Enter, Down | search |
| dialog.down / dialog.up | Down, Ctrl+J / Up, Ctrl+K | dialog |
| dialog.accept | Enter | dialog |
| dialog.nextAccount | Ctrl+Tab | dialog |
| help.close | q | help |

- Context bindings win over global ones.
- Global bindings must use Ctrl, Alt, or an F-key (or be `Escape`). This is a tested invariant.

`escape` resolution (`Navigation.escapeAction(state)` → action), first match wins:
1. help open → `close-help`
2. dialog open → `close-dialog`
3. search focused with text → `clear-search`
4. search focused, empty → `leave-search`
5. compose focused → `leave-compose` (pane = conversation)
6. pane = conversation → `close-conversation` (pane = list)
7. list with an open conversation → `close-conversation`
8. list with query → `clear-search`
9. otherwise → `hide-window`

### C7 · UI behaviour
- Window: `FloatingWindow` with title `OmaMessenger`, implicit size `Style.space(1120)×Style.space(760)`, min `Style.space(760)×Style.space(540)`. Content fills the window (no inset card).
- Columns: rail `Style.space(64)`; list `clamp(Style.space(260), 0.32 × (width − rail), Style.space(360))`; conversation takes the rest. Footer key-hint bar `Style.space(24)` high across the full width.
- Persisted in `service.uiState` (survives panel unload): `railKey`, `selectedId`, `activeId`, `pane`, `query`, `drafts{conversationId: text}`.
- Rail keys: `all`, `service:whatsapp`, `service:telegram`, `account:<id>`. Account entries appear under their service only when that service has more than 1 account.
- The cursor is a conversation **id**, never an index, so reordering never moves the selection to another chat.
- Opening a conversation: send `conversations.markRead` and `ui.setFocus`, focus the composer, restore its draft. Leaving or hiding sends `ui.setFocus{conversationId:"", windowActive:false}`.
- Conversation list and message list are `ListModel`s updated incrementally from events (`ListSync.planSync` ops); never replace the whole model on an event.
- Message list: `ListView` with `verticalLayoutDirection: ListView.BottomToTop`, newest at index 0. When within one viewport of the oldest message and `hasMore`, request `messages.list{before: oldestId}` and append. When a new message arrives while the view is at the newest end, keep it there.
- `open(payloadJson)`: call the base show logic; if the payload has `conversationId`, open it. If the window is already visible and `HYPRLAND_INSTANCE_SIGNATURE` is set, run `Hyprland.dispatch("focuswindow title:^OmaMessenger$")`. Never call `requestActivate` (F3).
- Demo mode: the rail shows a `DEMO` chip with tooltip "Seeded demo data. WhatsApp and Telegram are not connected."

### C8 · Visual and code rules
- No colour literals in `ui/` except `"transparent"`. Derive everything from F12 tokens.
- Row hover: `Style.hoverFill`. Row selected: `Util.alpha(Color.accent, Style.selectedFillAlpha)` plus a 2 px `Color.accent` bar on the left edge.
- Unread is a bold title plus a badge (bg `Color.accent`, text `Color.background`). A muted chat's badge uses bg `Util.alpha(Color.foreground, 0.25)`. Never rely on colour alone.
- Bubbles: outgoing bg `Util.alpha(Color.accent, 0.22)`; incoming bg `Util.alpha(Color.foreground, 0.06)`; text `Color.foreground`; max width 72 % of the pane. Failed messages show `Not sent · r to retry` in `Color.urgent`; clicking it retries.
- Status glyphs: pending `○`, sent `✓`, delivered `✓✓`, read `✓✓` in `Color.accent`, failed `!`.
- Service glyphs (Nerd Font FA code points): WhatsApp ``, Telegram ``, all/bar ``. Always pair them with a text label or tooltip.
- Message text: `TextEdit { readOnly: true; selectByMouse: true; textFormat: TextEdit.RichText }` fed `Format.linkify(Format.escapeHtml(text))`; `onLinkActivated: Qt.openUrlExternally(link)`.
- `ui/lib/*.js`: first line `.pragma library`; only `var` and `function` declarations at top level (so `tests/unit/load.cjs` can export them); no `import`/`require`.
- Every `.qml` under `ui/` passes Qt6 qmllint with the C9 import dir.

### C9 · Test commands

These are target commands for the completed Phase R/B harness. The current pre-R scaffold has a smaller Makefile; R01e and B19–B23 bring it into line before GR/GB.

- `make test`: `test-go test-js lint docs-check test-qml`. `test-e2e` (Docker) and `mutation` are separate.
- `test-go`: `go test -mod=vendor -race -coverprofile=build/cover.out ./backend/... ./tools/...`, then `go run -mod=vendor ./tools/covergate build/cover.out`. Gates live in `tools/covergate` as a table (single source; docscheck verifies this list matches):
  - 100 %: `domain`, `app/policy`
  - ≥ 90 %: `store`, `rpc`, `api`, `app`, `connector`, `connector/demo`, every `tools/*` package
  - ≥ 75 %: `notify`
  - `backend` (main): only `func main` is excluded, detected by AST.
- `test-js`: `node --test --experimental-test-coverage --test-coverage-include='ui/lib/**' --test-coverage-lines=95 --test-coverage-branches=90 tests/unit/` (Node ≥ 22.8), then `npm --prefix tools/uilint test`.
- `lint`:
  - `gofmt -l backend tools` (must be empty); `go vet -mod=vendor ./...`
  - `go run -mod=vendor ./tools/omalint ./backend/... ./tools/...`
  - `npm --prefix tools/uilint run lint`
  - `scripts/qml-imports.sh` (creates `build/qml/qs/{Commons,Ui}` symlinks to `${OMARCHY_PATH:-/usr/share/omarchy}/shell/…`)
  - `/usr/lib/qt6/bin/qmllint -I build/qml $(find ui -name '*.qml')` (settings in `ui/.qmllint.ini`)
  - `bash -n scripts/*.sh tests/*/*.sh`
- `docs-check`: `go test -mod=vendor ./tools/docscheck/ && node tools/uilint/gen-keys.cjs --check`.
- `test-qml`: `tests/qml/run.sh` (task B23).
- `mutation` (gate GQ only): mutation testing of `domain`, `store`, `app`, `app/policy`, `api` (task Q07).

### C10 · Architecture, complexity, coupling and cohesion rules (enforced in code after Phase R)

The table below is the target rule set. Phase A intentionally violates the `connector → store` and `rpc → app/store` edges and app size/cohesion limits; R01a–R05 add enforcement and remove these known violations before any UI phase begins.

Every rule below has exactly one enforcing check, named in the right-hand column. Thresholds live in code (`tools/omalint/rules/rules.go`, `tools/uilint/rules.cjs`). `tools/docscheck` fails if these tables and the code disagree.

**Layering** (allowed internal imports; stdlib always allowed; anything unlisted is a violation) — `omalint/layering`

| package | may import |
|---|---|
| `backend/internal/domain` | — |
| `backend/internal/store` | domain (+ `modernc.org/sqlite`) |
| `backend/internal/connector`, `connector/clocktest` | domain |
| `backend/internal/connector/demo` | connector, domain |
| `backend/internal/connector/connectortest` | connector, domain, connector/clocktest (added by D00) |
| `backend/internal/notify` | — |
| `backend/internal/app/policy` | domain |
| `backend/internal/app` | domain, connector, app/policy |
| `backend/internal/rpc` | — |
| `backend/internal/api` | app, rpc, domain |
| `backend` (main) | any internal package |
| `tools/*` | no `backend/internal/*` package except `tools/docscheck` → api (descriptors only) |

Test files (`_test.go`) may additionally import `store`, `notify`, `connector/clocktest`, `connector/demo` and `connector/connectortest`.

**Complexity and size** (non-test code)

| metric | Go — `omalint/size`, `omalint/complexity` | JS in `ui/lib` — ESLint | JS inside QML — `uilint/qml` |
|---|---|---|---|
| cyclomatic complexity per function | ≤ 10 | ≤ 8 | ≤ 8 |
| max nesting depth | ≤ 4 | ≤ 4 | ≤ 4 |
| function length (lines) | ≤ 60 | ≤ 50 | ≤ 40 |
| parameters | ≤ 5 | ≤ 5 | ≤ 5 |
| file length (lines) | ≤ 400 | ≤ 300 | QML file ≤ 350 |

Cyclomatic complexity = 1 + each `if`, `for`, `range`, non-default `case`, `comm` clause, `&&`, `||` (Go); ESLint's `complexity` rule (JS).

**Coupling** — `archtest` (tests over the real module using `golang.org/x/tools/go/packages`)
- Efferent coupling Ce (internal packages imported) ≤ 4 for every package except `backend` (main).
- Stable Dependencies Principle: for every internal edge A → B, instability I(B) ≤ I(A), where I = Ce / (Ca + Ce); a package with Ca + Ce = 0 has I = 0.
- No package named `util`, `utils`, `common`, `helpers`, `misc`, `shared` or `base`.
- Dependency inversion — `omalint/dip`: exported `New*` functions in `backend/internal/...` take parameters from other internal packages only when they are interfaces or `domain` value types (no `*store.Store`, `*connector.Manager`, ...). `main` is the only place where concrete types meet.
- Frozen interfaces — `omalint/frozeniface`: the method sets of `connector.Sink` (6 methods) and `connector.Connector` (4 methods) must equal the golden lists in `rules.go`. New capabilities are new optional interfaces (C4).

**Cohesion** — `archtest`
- LCOM4 = 1 for every struct type with ≥ 3 methods. Two methods are connected if they touch a common field or one calls the other. A type with LCOM4 > 1 does more than one job: split it.
- Every exported identifier in `backend/internal/...` is used outside its own package (in non-test code or by `main`); an export only its own package uses should be unexported.
- `archtest -update` writes `docs/ARCHITECTURE.md` (Mermaid dependency graph + per-package Ca, Ce, I, abstractness A, distance |A + I − 1| and LCOM4 table); the test fails when the file is stale.

**Privacy** — `omalint/nologcontent` (type-aware)
- No value of type `domain.Message`, `Contact`, `Conversation` or `Account`, and none of their fields `Text`, `Name`, `SenderName`, `Title`, `Preview`, `PreviewSender`, `Match`, may be passed to `log.*`, `slog.*`, `fmt.Print*`, or `fmt.Fprint*` to `os.Stderr`/`os.Stdout`.

**UI placement and style** — `uilint/qml` (+ ESLint for `ui/lib`)
- `ui/components/*.qml` must not contain `service` or `request(`. Only `ui/controllers/*.qml` and `ui/service/*.qml` may contain `request(`.
- `ui/lib/*.js` must not reference `Qt` except key constants in `Keymap.js`. It starts with `.pragma library` and uses ES5 syntax only (no `let`, `const`, arrows or template literals).
- No colour literals in `ui/` except `"transparent"` (C8); no `requestActivate`; relative imports and manifest entryPoints must exist.

## 3. Tasks

### Phase A — backend foundation

### [x] A01 · Rename Go module (S)
- files: go.mod
- accept: module is `github.com/timlittle/omamessenger`

### [x] A02 · Domain package (S)
- files: backend/internal/domain/domain.go
- accept: compiles; constants, types and `StatusAdvances`/`NormalizeOutgoingText` match C3/C5

### [x] A03 · Domain tests (S)
- deps: A02
- files: backend/internal/domain/domain_test.go
- do:
  1. Write a table-driven `TestStatusAdvances` covering every (from, to) pair over {pending, sent, delivered, read, failed, received}.
  2. Test `NormalizeOutgoingText`: trims, empty → ErrEmptyText, 4096 runes ok, 4097 → ErrTextTooLong, multibyte counted as runes.
  3. Test `ValidService`.
- accept: 100 % coverage of package
- verify: `go test -mod=vendor -cover ./backend/internal/domain/`

### [x] A04 · Store package (M)
- files: backend/internal/store/store.go
- accept: compiles; implements C2

### [x] A05 · Store tests (M)
- deps: A04
- files: backend/internal/store/store_test.go
- do: use `t.TempDir()`. One `t.Run` per case:
  1. Open creates the file with 0600 and the dir with 0700; reopening keeps data; `user_version == SchemaVersion`.
  2. Open fails on a DB whose `user_version` is higher than supported.
  3. Account upsert/validation/status; `SetAccountStatus` on an unknown id → ErrNotFound.
  4. `EnsureConversation` creates once (created=true), then updates title (created=false); missing fields error.
  5. `AddMessage`: unread +1 for incoming, +0 for outgoing; preview/sender/outgoing follow the newest message; an older backfilled message doesn't overwrite the preview; duplicate RemoteID returns inserted=false and doesn't change unread; unknown conversation → ErrNotFound; deleting an account cascades.
  6. Ordering: two messages with equal `Created` keep insertion order; a whole-second timestamp after a fractional one sorts after it (regression for the old RFC3339 text sort).
  7. `Messages` pagination: 120 msgs, limit 50 → 50 newest oldest-first with hasMore; `before` = oldest returned gives the next page; the last page has hasMore=false; unknown `before` → ErrNotFound.
  8. `Conversations(query)`: title match; message match fills `Match` with the newest matching text; `%` and `_` are literal; empty query returns all ordered by `last_activity` desc.
  9. `MarkRead` changed flag; `SetMuted`; `UnreadTotal` excludes muted.
  10. `UpdateMessageStatus` obeys `StatusAdvances` (`read`→`delivered` ignored).
  11. `Contacts` search is case-insensitive and sorted.
- accept: coverage ≥ 85 %
- verify: `go test -mod=vendor -race -cover ./backend/internal/store/`

### [x] A06 · Connector interfaces and Manager (M)
- deps: A04
- files: backend/internal/connector/connector.go, manager.go, clock.go, clocktest/clocktest.go, clocktest/clocktest_test.go, manager_test.go
- do:
  1. Declare the C4 interfaces. Add `RealClock` and `clocktest.Clock` (manual `Advance(d)` fires due callbacks in time order).
  2. `Manager{Store, Sink, Clock, Connectors}`. `Start(ctx)` upserts accounts and runs the C4 restart/backoff loop. `Send(ctx, conv, msg)` and `MarkRead` route by AccountID (unknown → error).
- accept: tests prove the backoff sequence 1,2,5,15,60,60 s using the fake clock; error status/detail set; ctx cancel stops all goroutines (`goleak`-free check via WaitGroup); routing errors for an unknown account.
- note: the fake clock has its own test file so its package coverage is measured instead of remaining at 0% under `go test -cover ./backend/internal/connector/...`.
- verify: `go test -mod=vendor -race -cover ./backend/internal/connector/...`

### [x] A07 · Notifier (S)
- files: backend/internal/notify/notify.go, notify_test.go
- do:
  1. `type Notifier interface{ Notify(title, body string) }`.
  2. `Desktop{}` runs `notify-send --app-name=OmaMessenger --category=im.received -- title body`. Call `cmd.Start()`, then `go cmd.Wait()` (no zombies). Ignore a missing binary.
  3. `Recorder` (tests) stores calls under a mutex.
- accept: test with a stub `notify-send` on PATH (via `t.Setenv`) that records its argv to a file; argv matches exactly; a missing binary doesn't panic.
- verify: `go test -mod=vendor -race -cover ./backend/internal/notify/`

### [x] A08 · App service (L)
- deps: A05, A06, A07
- files: backend/internal/app/app.go, app_test.go
- do:
  1. `App{Store, Manager, Notifier, Clock, Emit func(name string, data any)}`; it implements `connector.Sink`.
  2. Sink methods persist via the store and emit C3 events (`unread.changed` only when the total changes).
  3. Implement each C3 method as a Go method with typed params/results. Errors are typed: `ErrBadRequest`, `store.ErrNotFound`.
  4. `SendMessage`: normalize text, store an outgoing pending message with `Created = Clock.Now()`, emit `message.added` + `conversation.updated`, then `Manager.Send`.
  5. `Retry`: failed → pending, emit, resend.
  6. Apply the notification policy from C3. Settings default to all true.
- accept: tests using a fake connector + `notify.Recorder` + `clocktest`: send path statuses; retry; dedupe of incoming; notify suppressed for muted / focused-and-active (and the conversation marked read); preview hidden when `notificationPreview=false`; events emitted in order.
- verify: `go test -mod=vendor -race -cover ./backend/internal/app/`

### [x] A09 · RPC server (M)
- deps: A08
- files: backend/internal/rpc/server.go, server_test.go, fuzz_test.go
- do:
  1. `Serve(ctx, r io.Reader, w io.Writer, h Handler) error`, where `Handler` maps method → `func(ctx, json.RawMessage) (any, error)`.
  2. `bufio.Scanner` with a 1 MiB buffer; each request handled in its own goroutine; writes serialized by a mutex; `Emit(name, data)` writes event lines.
  3. Map errors to C3 codes; malformed JSON → `bad_request` with id 0; returns nil on EOF.
  4. `Register(app)` builds the C3 method table.
- accept: tests over `io.Pipe` for every method's happy path and error codes; concurrent requests produce no interleaved lines (`-race`); `FuzzDecode` never panics.
- verify: `go test -mod=vendor -race -cover ./backend/internal/rpc/ && go test -mod=vendor -run=^$ -fuzz=FuzzDecode -fuzztime=10s ./backend/internal/rpc/`

### [x] A10 · Demo connector (L)
- deps: A06
- files: backend/internal/connector/connector.go, manager.go, backend/internal/connector/demo/demo.go, seed.go, demo_test.go, backend/internal/app/app.go, app_test.go, backend/internal/rpc/server_test.go
- do: implement C5 exactly. `New(clock, rand *rand.Rand, chatter bool) []connector.Connector`; `SetChatter(bool)`; `Inject(conversationRemoteID string)`.
- accept: tests with `clocktest` + a recording Sink:
  - Seeding twice yields identical store state.
  - The unread counts in the C5 table hold.
  - Status timeline for a send at exact offsets; Sam's first-attempt failure, then retry success.
  - Reply + typing events for DMs only.
  - Chatter fires only when enabled.
  - Inject works.
- note: added the optional `HistorySink` path so initial/backfilled incoming messages persist without triggering live desktop notifications; Manager forwards it and App implements it.
- note: `demo.inject` takes a local `Conversation.id`; App resolves it to the connector's remote ID. The RPC integration case exercises that C3 shape.
- verify: `go test -mod=vendor -race -cover ./backend/internal/connector/demo/`

### [x] GA · Phase A gate (S)
- deps: A01–A10
- files: docs/TASKS.md, README.md, CONTRIBUTING.md, AGENTS.md, FORGE_SPEC.md, .agents/README.md, Makefile, scripts/install-local.sh, scripts/test-docker.sh, scripts/build-release.sh, bin/oma-messenger-service-linux-amd64, bin/oma-messenger-service-linux-arm64
- checks: D1; pre-R D2 (`gofmt -l backend`, `go vet -mod=vendor ./backend/...`); pre-R D3 (`go test -mod=vendor -race -count=3 ./backend/...`); D6, D7 (`FuzzDecode`), D8, D9 and pre-R D11. D2/D3 over `tools/...`, D4, D5, D10 and architecture conformance in D11 are enforced at GR after R01a–R01e and R08. For now, record `go test -mod=vendor -cover ./backend/...` per-package numbers in the gate note.
- plus:
  - GA1 Order independence: `go test -mod=vendor -race -shuffle=on -count=3 ./backend/...` passes.
  - GA2 Demo determinism: `go test -mod=vendor -race -count=5 ./backend/internal/connector/demo/` passes.
  - GA3 Ordering regression: `go test -mod=vendor -run 'Order' -v ./backend/internal/store/` shows the whole-second-after-fraction case passing.
  - GA4 Privacy: `git grep -nE '(log|fmt)\.(Print|Fatal|Fprint)[a-z]*\(.*\.(Text|Name|SenderName|Title)' -- backend` is empty.
- note: Current phase-A verification: Go has 112 passing test cases (`go test -mod=vendor -json ./backend/...`); Node has 2 passing tests (`node --test tests/unit/*.test.cjs`). Aggregate core coverage is 85.7%; package statement coverage: app 91.4%, connector 88.4%, clocktest 92.5%, demo 88.8%, domain 100%, notify 100%, rpc 82.5%, store 85.8%; `backend` main package is excluded from the package gates. `make test` passes. Pre-R D2/D3 (three race runs), D6, D7 (30 s `FuzzDecode`), GA1–GA4 and D9 pass. D4/D5/D10 and tool-package D2/D3 are deferred to GR. D8 passed in a clean checkout at `1bc3ca3`: Go tests (`-race -count=3`), Node tests, formatting and `go vet`. Current docs describe stdio JSON-lines IPC and the seeded demo, not the removed HTTP/token API. Replacing two `mktemp` templates that produced false `XXX` matches made the prescribed D9 scan meaningful.

  D11 review table (reviewed against code commit `c4c6975`; the C10 target rules are explicitly identified as Phase R work):

  | doc | reviewed at commit | result | what changed |
  |---|---|---|---|
  | README.md | `c4c6975` | updated | Describes JSON-lines IPC, demo mode, user data paths and old scaffold cleanup. |
  | CONTRIBUTING.md | `c4c6975` | updated | Test map describes JSON-lines RPC and SQLite behavior tests. |
  | AGENTS.md | `c4c6975` | updated | Layout follows C1 and labels root-level scaffold files as transitional. |
  | FORGE_SPEC.md | `c4c6975` | updated | Describes stdio IPC and the current demo versus real-service gap. |
  | .agents/README.md | `c4c6975` | updated | Removes the claim that `make test` runs Docker UI checks. |
  | docs/TASKS.md (§0–§2) | `c4c6975` | updated | Marks C1/C9/C10 as target architecture and scopes GA to pre-R behavior. |
  | docs/KEYS.md | n/a | n/a | Created in B15. |
  | docs/PROTOCOL.md | n/a | n/a | Created in R08. |
  | docs/ARCHITECTURE.md | n/a | n/a | Created in R01d. |

  The current Makefile's Go targets now run the present backend packages instead of the removed scaffold HTTP tests; its 80% aggregate coverage command covers all backend packages. Both bundled architecture binaries were rebuilt from the JSON-lines helper with `make build-all`.

### Phase R — refactor to the architecture rules (C10)

Why: A06/A08/A09 work but break C10, and C10 must be enforced by code before more code is written. `connector` imports `store`, `rpc` imports `app` and `store`, and `app.go` mixes six responsibilities. Fix this before anything else builds on these packages. Refactors keep behaviour identical: move tests with the code and keep their assertions.

### [x] R01a · omalint framework, layering and size rules (M)
- deps: GA
- files: tools/omalint/main.go, tools/omalint/rules/rules.go, tools/omalint/analyzers/layering/, tools/omalint/analyzers/size/, tools/omalint/suppress/, `testdata/` under each analyzer, go.mod, go.sum, vendor/
- do:
  1. `go get golang.org/x/tools@v0.36.0` (latest release declaring `go 1.23.0`), then `go mod vendor`.
  2. `main.go` runs the analyzers with `multichecker.Main`.
  3. `rules.go` holds the C10 layering table, test-file extras and size limits as Go data (the single source).
  4. `layering` reports a forbidden import at the import spec position.
  5. `size` reports file length, function length and parameter count over the limits. It skips `_test.go` and generated files.
  6. Implement the suppression comment (§0 rule 10), shared by every analyzer, in `tools/omalint/suppress/`. A missing reason is itself a finding.
- accept:
  - `analysistest` tests per analyzer cover an allowed case, a violating case at threshold+1, a case exactly at the threshold, and a suppressed case (with and without reason).
  - Run on the repo now, it reports at least: `connector` imports `store`; `rpc` imports `app`; `rpc` imports `store`; `app/app.go` > 400 lines. Paste the output into the task note.
- result: Go 1.23.12 analyzer tests pass. Coverage: omalint 100%, layering 93.1%, size 81.1%, suppress 84.6%.
- current findings (`go run -mod=vendor ./tools/omalint ./backend/...`):
  ```text
  backend/internal/connector/manager.go:11:2: layering: connector may not import store
  backend/internal/app/app.go:14:2: layering: app may not import notify
  backend/internal/app/app.go:15:2: layering: app may not import store
  backend/internal/rpc/server.go:14:2: layering: rpc may not import app
  backend/internal/rpc/server.go:15:2: layering: rpc may not import store
  backend/internal/rpc/server_test.go:19:2: layering: rpc test may not import app
  backend/internal/rpc/server_test.go:20:2: layering: rpc test may not import connector
  backend/internal/rpc/server_test.go:22:2: layering: rpc test may not import domain
  backend/internal/store/store.go:5:1: size: 510 lines; maximum 400
  backend/internal/app/app.go:3:1: size: 492 lines; maximum 400
  backend/internal/rpc/server.go:94:1: size: function has 78 lines; maximum 60
  backend/main.go:31:9: size: function has 6 parameters; maximum 5
  ```
- verify: `go test -mod=vendor ./tools/omalint/... && (go run -mod=vendor ./tools/omalint ./backend/...; test $? -ne 0)`

### [x] R01b · Complexity analyzers (S)
- deps: R01a
- files: tools/omalint/analyzers/complexity/ (+ testdata), tools/omalint/main.go, tools/omalint/rules/rules.go
- do: cyclomatic complexity and nesting depth per C10, for function declarations and function literals separately.
- accept: testdata functions at exactly the limit pass and at limit+1 fail, for each counted construct (`if`, `for`, `range`, `case`, `select` case, `&&`, `||`, nested func literal).
- result: Go 1.23.12 tests pass; complexity analyzer coverage is 94.5%. Fixtures include four-level nesting at the limit, a five-level violation, valid suppression and missing-reason rejection.
- verify: `go test -mod=vendor ./tools/omalint/...`

### [ ] R01c · Design analyzers: dip, frozeniface, nologcontent (M)
- deps: R01a
- files: tools/omalint/analyzers/{dip,frozeniface,nologcontent}/ (+ testdata), tools/omalint/main.go, tools/omalint/rules/rules.go
- do: implement the three rules exactly as C10 states. Use `pass.TypesInfo` (type-aware, not name matching) for `dip` and `nologcontent`.
- accept: each analyzer has testdata with violating, allowed and suppressed cases; `frozeniface` fails when a method is added to or removed from the testdata copy of `Sink`.
- verify: `go test -mod=vendor ./tools/omalint/...`

### [ ] R01d · archtest: coupling and cohesion tests (M)
- deps: R01a
- files: backend/internal/archtest/arch_test.go, backend/internal/archtest/metrics_test.go, backend/internal/archtest/doc.go (package comment only), docs/ARCHITECTURE.md
- do:
  1. Load `./backend/...` with `golang.org/x/tools/go/packages` (syntax + types).
  2. Compute per package: Ca, Ce, I, abstractness A (exported interfaces / exported named types), distance |A + I − 1|, and LCOM4 per struct type (union-find over methods connected by shared field selectors on the receiver or receiver-method calls).
  3. Tests:
     - `TestEfferentCoupling`, `TestStableDependencies`, `TestNoJunkDrawerPackages`
     - `TestLCOM4`, `TestExportsUsedOutsidePackage`
     - `TestSuppressionBudget` (≤ 5 `omalint:ignore` across the module)
     - `TestArchitectureDocCurrent`, with an `-update` flag to regenerate docs/ARCHITECTURE.md
  4. Metric functions are pure and unit-tested with small synthetic packages in `testdata/`.
- accept: the metric unit tests pass. Run on the repo now, the coupling/cohesion tests fail exactly for the known problems (app LCOM4 or size, rpc/connector edges); paste them into the note. They must pass after R05.
- verify: `go test -mod=vendor -run 'Metric|Lcom|Instability' ./backend/internal/archtest/`

### [ ] R01e · covergate (S)
- deps: GA
- files: tools/covergate/main.go, tools/covergate/gate.go, tools/covergate/gate_test.go, Makefile (`test-go`)
- do: parse a cover profile; aggregate statements per package directory; exclude only the statements inside `func main` of `backend/main.go`, found via `go/parser`; compare against the C9 table in code; print a table; exit 1 on any miss. Packages missing from the table fail ("no gate defined").
- accept: tests with synthetic profiles cover pass, fail, a missing gate and main exclusion.
- verify: `go test -mod=vendor -cover ./tools/covergate/`

### [ ] R02 · ErrNotFound lives in domain (S)
- deps: R01a, R01b, R01c, R01d, R01e
- files: backend/internal/domain/domain.go, domain_test.go, backend/internal/store/store.go
- do: add `var ErrNotFound = errors.New("not found")` to domain. In store, set `var ErrNotFound = domain.ErrNotFound`, so existing `errors.Is` checks keep working.
- accept: a store test asserts `errors.Is(err, domain.ErrNotFound)` for a missing conversation.
- verify: `go test -mod=vendor -race ./backend/...`

### [ ] R03 · Manager depends on AccountStore (S)
- deps: R02
- files: backend/internal/connector/connector.go, manager.go, manager_test.go
- do: declare `AccountStore` exactly as in C4. The Manager takes it instead of `*store.Store`. Tests use an in-memory fake (`map` + mutex) instead of SQLite.
- accept: `go list -mod=vendor -f '{{join .Imports " "}}' ./backend/internal/connector` doesn't contain `/store`; all manager tests pass unchanged in intent.
- verify: `go test -mod=vendor -race -cover ./backend/internal/connector/`

### [ ] R04 · Generic rpc + api adapter (M)
- deps: R02
- files: backend/internal/rpc/server.go, server_test.go, fuzz_test.go; create backend/internal/api/api.go, api_test.go
- do:
  1. Remove the `app` and `store` imports from `rpc`. `Serve` takes `Handler` plus `type ErrorCoder func(error) (code, message string)`. When the coder is nil, every error becomes `internal` / `"internal error"`. Keep framing, concurrency, `Stream.Emit` and `FuzzDecode` in `rpc`.
  2. In `api`:
     - Declare `type Commands interface { ... }` listing exactly the app methods used.
     - `Register(c Commands) rpc.Handler` builds the C3 method table, decoding params into the app param structs.
     - `Code(err)` maps `domain.ErrNotFound` → `not_found` and `app.ErrBadRequest` → `bad_request` (message = err text); anything else → `internal` with message `"internal error"` (never leak internal text).
  3. Move method-level tests from `rpc` to `api_test.go` using a fake `Commands`. `rpc` tests cover only framing, errors, ids, concurrency and EOF.
- accept: `rpc` imports stdlib only; every C3 method has a happy-path and an error-path test in `api_test.go`; demo-only `demo.inject` is registered only when the Commands value reports demo mode.
- verify: `go test -mod=vendor -race -cover ./backend/internal/rpc/ ./backend/internal/api/`

### [ ] R05 · Split app by responsibility (L)
- deps: R03, R04
- files: backend/internal/app/{app.go,ports.go,commands.go,ingest.go,session.go,*_test.go}; create backend/internal/app/policy/{policy.go,policy_test.go}
- do:
  1. `ports.go` declares consumer-side interfaces:
     - `Repository`: only the store methods app calls.
     - `Dispatcher`: `Send` and `MarkRead`.
     - `Notifier`: `Notify(title, body string)`.
     - `type Emit func(name string, data any)`.
     - `DemoInjector`: optional; app checks for it with a type assertion.
  2. `session.go`: `Session` holds settings, focus (`conversationId`, `windowActive`) and the last unread total, behind one mutex. It exposes `UnreadChanged(repo) (int, bool)`.
  3. `commands.go`: `type Commands` implements every C3 method (UI → helper).
  4. `ingest.go`: `type Ingest` implements `connector.Sink` and `connector.HistorySink` (connectors → helper). It uses `policy` for notify decisions.
  5. `policy` package (pure, no I/O): `type Input struct{ Notifications, Preview, Muted, Focused, WindowActive bool; Kind, Sender, Title, Text string }`, `ShouldNotify(Input) bool`, `MarkReadOnArrival(Input) bool`, `Notification(Input) (title, body string)`, all per C3.
  6. `app.go` keeps only `New(repo Repository, d Dispatcher, n Notifier, clock connector.Clock, emit Emit) (*Commands, *Ingest)`.
- accept:
  - omalint and archtest report nothing for `app` (size, complexity, dip, LCOM4).
  - `app` imports only domain, connector and app/policy (no `store`, no `notify`).
  - `policy_test.go` is a table over all 2^5 boolean combinations × {direct, group} and reaches 100 % coverage.
  - Previous app tests still pass, moved into `commands_test.go` / `ingest_test.go`, with a fake Repository or a real store from a test-only import.
- verify: `go test -mod=vendor -race -cover ./backend/internal/app/... && go run -mod=vendor ./tools/omalint ./backend/... && go test -mod=vendor ./backend/internal/archtest/`

### [ ] R06 · Helper main (M)  (formerly A11)
- deps: R05, A10
- files: backend/main.go (rewrite), backend/config.go, backend/main_test.go (replace)
- do:
  1. `resolveConfig(args []string, env func(string) string) (Config, error)`. Flags: `--demo`, `--no-chatter`, `--seed`, `--data-dir`, `--db`, `--version`. Paths follow C2.
  2. `main` wires store → Manager(AccountStore=store) → `app.New` → `api.Register` → `rpc.Serve` over stdin/stdout, with `emit` writing via `rpc.Stream.Emit`. Connectors: demo only when `--demo`; none otherwise. Logs one start line to stderr (version, demo on/off; no paths to message content).
  3. Exit 0 on stdin EOF or SIGTERM. Delete all HTTP/token code.
- accept:
  - `resolveConfig` table tests cover every flag, the XDG fallbacks and invalid input.
  - Integration test: build the binary into `t.TempDir()`; run it with `--demo --no-chatter --seed 1 --data-dir $TMP`; send `hello` and `conversations.list` → 11 conversations; send `demo.inject`; close stdin → exit 0 within 2 s.
  - Privacy test: the captured stderr from that run contains none of the C5 seed message texts, contact names or conversation titles.
- verify: `go test -mod=vendor -race ./backend/`

### [ ] R07 · Launcher prefers dev build (S)  (formerly A12)
- deps: GA
- files: bin/oma-messenger-service, .gitignore, tests/unit/launcher.test.cjs
- do: if `$bin_dir/dev/oma-messenger-service` is executable, `exec` it; else keep the arch selection. Add `/bin/dev/` and `/build/` to .gitignore.
- accept: the test runs the script with a temp bin dir containing a stub dev binary and asserts it was chosen; without the stub it picks the arch binary; with neither it exits 1 with a message.
- verify: `node --test tests/unit/launcher.test.cjs`

### [ ] R08 · Docs checks in code (M)
- deps: R04, R06, R01a, R01e
- files: tools/docscheck/{main.go,check.go,protocol.go,check_test.go}, backend/internal/api/descriptors.go, docs/PROTOCOL.md, Makefile (`docs-check`)
- do:
  1. `api` exposes `Methods() []MethodDesc{Name, Params, Result, Errors}` and `Events() []EventDesc{Name, Data, When}` (descriptors only, no behaviour). `go run ./tools/docscheck -update` renders docs/PROTOCOL.md from them.
  2. Checks, as tests in `check_test.go` that run over the real repo:
     - `TestProtocolDocCurrent`: docs/PROTOCOL.md equals the rendering.
     - `TestContractC3MatchesAPI`: method and event names in the C3 tables of docs/TASKS.md equal the descriptors, both ways.
     - `TestContractC10MatchesRules`: the layering table equals `rules.go`.
     - `TestContractC9MatchesCovergate`: the gates list equals `tools/covergate`.
     - `TestLinksResolve`: relative markdown links in the inventory docs (§0.1 D11) resolve.
     - `TestPathsExist`: backticked repo paths starting with `ui/`, `backend/`, `scripts/`, `tools/`, `tests/`, `docs/`, `bin/` in those docs exist (patterns with `*` or `{}` must match ≥ 1 file). docs/TASKS.md is excluded because it names planned files.
     - `TestMakeTargetsExist`: every `make <target>` mentioned exists in the Makefile.
     - `TestFlagsExist`: every `--flag` mentioned in README/CONTRIBUTING is defined in `backend/config.go` (parsed with `go/ast`).
     - `TestKeysBlockCurrent`: when README has `<!-- keys:start -->`/`<!-- keys:end -->` markers, the block equals the table in docs/KEYS.md.
  3. Each check function is unit-tested on small fixture docs in `testdata/` (pass and fail cases).
- accept: `go test ./tools/docscheck/` passes on the repo; fixture tests prove each check fails on bad input.
- verify: `go test -mod=vendor -cover ./tools/docscheck/`

### [ ] GR · Phase R gate (S)
- deps: R01a–R01e, R02–R08
- checks: all of §0.1 (D1–D11).
- plus:
  - GR1 Test count ≥ GA's recorded count.
  - GR2 Binary smoke: `go build -mod=vendor -o bin/dev/oma-messenger-service ./backend && printf '{"id":1,"method":"hello","params":{}}\n' | bin/dev/oma-messenger-service --demo --no-chatter --data-dir "$(mktemp -d)"` prints one response line containing `"protocol":1` and exits 0.
  - GR3 `go test -mod=vendor -race -shuffle=on -count=3 ./backend/...` passes.
  - GR4 Offline: `docker run --rm --network none -v "$PWD":/src -w /src golang:1.23 go test -mod=vendor ./backend/...` passes (no test touches the network).
- note: record test counts and the coverage table.

### Phase B — UI on demo data

### [ ] B01 · JS test loader (S)
- deps: GR
- files: tests/unit/load.cjs, tests/unit/load.test.cjs
- do: `load("lib/Keymap.js")` reads `ui/<path>`, strips the `.pragma library` line, resolves `.import "X.js" as X` recursively, evaluates in `new Function`, and returns an object of all top-level `function` and `var` names.
- verify: `node --test tests/unit/`

### [ ] B02 · Rpc.js (S)
- deps: B01
- files: ui/lib/Rpc.js, tests/unit/rpc.test.cjs
- do: `encodeRequest(id, method, params)` → line with `\n`; `parseLine(line)` → `{kind:"response",id,result,error}` | `{kind:"event",name,data}` | `{kind:"invalid"}`; `errorText(error)` → user string.
- accept: invalid JSON, missing fields and arrays → invalid.

### [ ] B03 · Keymap.js (M)
- deps: B01
- files: ui/lib/Keymap.js, tests/unit/keymap.test.cjs
- do:
  1. `BINDINGS` array implementing the C6 table: `{action, keys:[...], contexts:[...], label, hint:bool, demoOnly:bool}`.
  2. `match(context, key, modifiers, text, demo)` → action or "" (context bindings first, then global).
  3. `bindingsFor(context)` for hints; `helpSections()` for the help sheet. Qt key/modifier constants are defined in the file.
- accept:
  - Every C6 row matches.
  - Uppercase/Shift rules; `?` matches with Shift held.
  - `j` does not match in compose or search.
  - Ctrl+K in dialog → `dialog.up`.
  - Invariant test: no global binding is a plain printable key.
  - The set of actions in `BINDINGS` equals the set of keys in `Actions.OWNERS` (B15a): no unbound owner entries and no unowned bindings.

### [ ] B04 · Format.js (S)
- deps: B01
- files: ui/lib/Format.js, tests/unit/format.test.cjs
- do:
  - `escapeHtml`; `linkify(escaped)` for http(s) URLs only, without breaking entities.
  - `highlight(escaped, query)` wraps case-insensitive matches in `<b>`.
  - `initials(name)` → max 2 letters, Unicode-safe; empty → "?".
  - `timeLabel(ms, nowMs)` → `HH:mm` today, `Yesterday`, weekday within 6 days, else `d MMM` (or `d MMM yyyy` for another year).
  - `dayLabel(ms, nowMs)` → Today / Yesterday / `Monday` / `12 September 2026`.
  - `statusGlyph(status)` per C8; `previewLine(conv)` → `You: …` / `Priya: …` (groups) / text.
- accept: tests use a fixed `nowMs` and local-time-independent assertions (construct dates with `new Date(y,m,d,h,mi)`).

### [ ] B05 · UI logic libraries (M)
- deps: B01, B03
- files: ui/lib/Rail.js, ui/lib/Selection.js, ui/lib/ListSync.js, ui/lib/Timeline.js, ui/lib/Navigation.js, and one `tests/unit/<name>.test.cjs` per library
- do: one responsibility per file, each ≤ 300 lines:
  - `Rail.js`: `items(accounts, conversations)` → C7 rail list with unread sums (muted excluded) and worst account status; `filter(conversations, railKey)`; `next(items, key, delta)` (wraps).
  - `Selection.js`: `move(ids, selectedId, delta)` (clamped; missing selection → first); `edge(ids, "top"|"bottom")`; `nextUnread(conversations, selectedId)` (wraps; skips muted).
  - `ListSync.js`: `planSync(oldIds, newIds)` → minimal ops `[{op:"remove",index},{op:"insert",index,id},{op:"move",from,to}]`; `upsertById(list, item, compareFn)`.
  - `Timeline.js`: `annotate(newestFirst, isGroup, nowMs)` → per message `{showDay, dayLabel, showSender, groupedWithOlder}`; `annotateAt(newestFirst, index, isGroup, nowMs)` for single-item updates.
  - `Navigation.js`: `keyContext(state)` per the C6 precedence; `escapeAction(state)` per C6.
- accept:
  - `planSync` property test: 500 random (old, new) pairs, including duplicates removed and empty lists; applying the ops to old yields new.
  - `escapeAction` and `keyContext` table tests cover every row of C6.
  - `Timeline` tests cover day changes, sender changes in groups, and none of that for direct chats.
  - C9 JS coverage gates hold per file.
- verify: `node --test --experimental-test-coverage --test-coverage-include='ui/lib/**' --test-coverage-lines=95 --test-coverage-branches=90 tests/unit/`

### [ ] B06 · Service layer (M)
- deps: GR, B02
- files: ui/Service.qml, ui/service/HelperProcess.qml, ui/service/RpcClient.qml, ui/service/AppState.qml
- do:
  1. `HelperProcess.qml` (process only):
     - Properties `status` (`starting|ready|stopped|error|missing`), `detail`; functions `start()`, `stop()`, `write(line)`; `signal line(string text)`.
     - Process per F10/F11 with args `["--demo"]`; the binary path is resolved relative to the plugin root.
     - Restart on unexpected exit after 1 s, 3 s, 10 s. After 5 exits within 60 s → `error` with the last stderr line as `detail`. Binary missing → `missing`.
  2. `RpcClient.qml` (protocol only):
     - `property var transport` (anything with `write(line)` and a `line` signal).
     - `request(method, params, callback(error, result))` with a pending map and 15 s timeout; `signal event(string name, var data)`; on transport restart, fail pending requests.
     - Uses `Rpc.js`.
  3. `AppState.qml` (state only): `accounts`, `unreadTotal`, `demo`, `version`, `uiState` (C7 defaults); `apply(name, data)` updates state from events.
  4. `Service.qml` (composition only, F7, ≤ 80 lines):
     - Instantiates the three and exposes `status`, `detail`, `demo`, `unreadTotal`, `accounts`, `uiState`, `windowOpen`, `request()`, `event` and `applySettings(settings)` (→ `settings.apply`).
     - On ready, sends `hello` and seeds AppState.
- accept:
  - `RpcClient` works with a fake transport. Harness scenario S13 (B23) uses a QML fake transport to check timeout, error mapping and that events never resolve a pending request.
  - Covered by B23 scenarios S1, S9, S13.

### [ ] B07 · Manifest (S)
- files: manifest.json
- do: kinds `["service","panel","bar-widget"]`; entryPoints per C1; version `0.2.0`; author "Tim Little"; homepage = GitHub URL. `barWidget`: displayName "OmaMessenger", category "Communication", defaultSection "right", schema booleans `notifications` (true), `notificationPreview` (true), `demoChatter` (true) with labels/descriptions.
- verify: `omarchy plugin validate .`

### [ ] B08 · Small components (S)
- deps: B04
- files: ui/components/Avatar.qml (initials circle, `name`, `size`), ui/components/UnreadBadge.qml (`count`, `muted`; hidden at 0; "99+" cap), ui/components/ServiceGlyph.qml (`service`)
- accept: C8 colours; qmllint clean

### [ ] B09 · ServiceRail.qml (M)
- deps: B05, B08
- files: ui/components/ServiceRail.qml
- do: props `items` (from `Rail.items`), `selectedKey`, `demo`. Signals `selected(key)`, `newChat()`, `help()`. Each item shows glyph, label, unread badge and status dot (`connecting` = hollow ring, `error`/`needs-auth` = `Color.urgent` dot) with a tooltip. Bottom: `+` (New chat · Ctrl+N), `?` (Shortcuts · F1). DEMO chip per C7.

### [ ] B10 · Conversation list (M)
- deps: B04, B08
- files: ui/components/ConversationRow.qml, ui/components/ConversationList.qml
- do:
  - Row: avatar, title (bold when unread, elided), time label, preview line (or highlighted `match` when searching), service glyph + account name when the service has >1 account, mute icon ``, badge; selected/hover per C8.
  - List: `ListView` over a `ListModel`; props `model`, `selectedId`, `query`; signals `activated(id)`; `positionOn(id)`.
  - Empty states: "No conversations yet · Ctrl+N to start one" / "No chats match “q”".
- accept: long-title elision verified in the B23 S6 screenshot-free geometry check (title.width ≤ row.width).

### [ ] B11 · MessageDelegate.qml (M)
- deps: B04
- files: ui/components/MessageDelegate.qml
- do: day separator (when `showDay`), sender name for incoming group messages when `showSender`, bubble per C8, time + status glyph row, failed retry link. Tighter top spacing when `groupedWithOlder`. Signal `retry(id)`.

### [ ] B12 · Composer.qml (S)
- files: ui/components/Composer.qml
- do: `TextArea` with wrap, grows to 6 lines then scrolls, placeholder "Message <title>". Props `text`, `enabled`; signal `submitted(text)`. Keys are forwarded by Panel (do not handle Enter locally except via Panel routing). Send button.

### [ ] B13 · ConversationView.qml (M)
- deps: B11, B12
- files: ui/components/ConversationView.qml
- do: header (avatar, title, subtitle `Service · Account · N members` or `<name> is typing…` or account status), BottomToTop message `ListView` per C7, composer, empty state "Pick a chat · j/k to move · Enter to open". Functions `scrollBy(lines)`, `scrollPage(dir)`, `scrollToNewest()`, `scrollToOldest()`. Signals `loadOlder()`, `retry(id)`, `send(text)`.

### [ ] B14 · NewChatDialog.qml (M)
- deps: B04
- files: ui/components/NewChatDialog.qml
- do: modal surface inside the window (`BorderSurface` + scrim). Account chooser (one button per account; Ctrl+Tab cycles), search field, filtered contact list (`contacts.list` results passed in), Enter opens. Signals `accountChanged(id)`, `queryChanged(q)`, `accepted(accountId, contactId)`, `cancelled()`.

### [ ] B15 · Help and hints (S)
- deps: B03, B22
- files: ui/components/ShortcutHelp.qml, ui/components/KeyHints.qml, tools/uilint/gen-keys.cjs, tools/uilint/test/gen-keys.test.cjs, docs/KEYS.md, README.md (keys block)
- do:
  1. Help overlay from `Keymap.helpSections()`; footer hints from `Keymap.bindingsFor(context)` where `hint`.
  2. `gen-keys.cjs` writes docs/KEYS.md and the README block between `<!-- keys:start -->` and `<!-- keys:end -->`. `--check` exits 1 when either is stale.
- accept: the generator test covers write, check-pass and check-fail; `node tools/uilint/gen-keys.cjs --check` passes.
- verify: `npm --prefix tools/uilint test && node tools/uilint/gen-keys.cjs --check`

### [ ] B15a · Controllers and action registry (M)
- deps: B05, B06
- files: ui/lib/Actions.js, tests/unit/actions.test.cjs, ui/controllers/ListController.qml, ui/controllers/ConversationController.qml, ui/controllers/DialogController.qml, ui/controllers/WindowController.qml
- do:
  1. `Actions.js`: `OWNERS` maps every C6 action to one of `"list"`, `"conversation"`, `"dialog"`, `"window"`; `owner(action)`.
  2. Each controller is a non-visual `QtObject`/`Item` with `property var service` and `function handles(action)` / `function run(action)`. Controllers are the only UI code that calls `service.request` (C10):
     - `ListController`: rail key, query, visible conversations `ListModel`, selection, unread jump, mute, search via `conversations.list`, event handling for `conversation.updated`.
     - `ConversationController`: active conversation, messages `ListModel` (BottomToTop), paging, send, retry, markRead, `ui.setFocus`, drafts, typing state, `message.added`/`message.updated`/`typing` events.
     - `DialogController`: new-chat dialog state, `contacts.list`, `conversations.open`.
     - `WindowController`: help, hide window, demo inject, rail switching.
  3. Controllers read and write `service.uiState` per C7 so state survives panel unload.
- accept:
  - `actions.test.cjs`: every `OWNERS` action is bound in `Keymap.BINDINGS`, and vice versa.
  - Each controller file is ≤ 350 lines and contains no visual items.
  - Covered by B23 scenarios S2–S8, S10, S11.

### [ ] B16 · Panel.qml (M)
- deps: B06, B09–B15, B15a
- files: ui/Panel.qml
- do:
  1. Compose the window per C7 and instantiate the four controllers, passing `service`.
  2. One `routeKey(event)` serves the root key catcher and the search/composer/dialog fields (`Keys.priority: Keys.BeforeItem`):
     - `action = Keymap.match(Navigation.keyContext(state), …)`, then the controller named by `Actions.owner(action)` runs it.
     - Accept the event only if an action ran.
  3. Bind views to controller properties and connect view signals to controller functions.
  4. Implement `open(payloadJson)` / `close()` per F5 and C7; `onClosing` → `shell.hide`. No `request(` calls and no business logic here.
- accept: ≤ 350 lines; B23 scenarios S1–S13 pass.

### [ ] B17 · BarWidget.qml (S)
- deps: B06
- files: ui/BarWidget.qml
- do: `BarWidget` + `BarIconButton` with glyph `` and a count bubble when `unreadTotal > 0`; glyph dimmed when service status ≠ ready; tooltip `OmaMessenger · N unread` (+ "· demo"); left click → toggle (F6); `onSettingsChanged`/`Component.onCompleted` → `applySettings`.

### [ ] B18 · Remove scaffold files (S)
- deps: B16, B17
- files: delete Panel.qml, Service.qml, keyboard.js, tests/unit/keyboard.test.cjs, tests/e2e/mocks/, scripts/check-coverage.py, scripts/install-local.sh
- verify: `git grep -n "keyboard.js\|api.token\|43821"` returns nothing outside docs/TASKS.md and README's migration note

### [ ] B19 · Makefile and install scripts (M)
- deps: GR, B18
- files: Makefile, scripts/install.sh, scripts/link.sh, scripts/build-release.sh
- do: targets (each with a `##` help text):
  - `build` → bin/dev/oma-messenger-service for the host arch.
  - `build-all` → release binaries.
  - `install` → build; stage `manifest.json LICENSE README.md ui/ bin/` into a temp dir; back up an existing plugin dir to `~/.config/omarchy/plugin-backups/<id>-<timestamp>` (outside the plugins dir); rsync with `--delete`; `omarchy plugin validate`; `omarchy-shell shell rescanPlugins`; `omarchy plugin enable <id>`; `omarchy-shell shell summon <id> '{}'` unless `OPEN=0`.
  - `link` → same backup, then symlink the checkout (live reload).
  - `open`.
  - `uninstall` → `omarchy plugin remove <id> --yes`.
  - `demo-reset` → delete `${XDG_DATA_HOME:-~/.local/share}/omamessenger/demo.db*` after an interactive confirmation (skip it with `YES=1`).
  - `test test-go test-js lint test-qml test-e2e fuzz clean` per C9.
  - Refuse any target dir other than `~/.config/omarchy/plugins/io.github.omamessenger`.
- accept: every `omarchy`/`omarchy-shell` call goes through the `OMARCHY`/`OMARCHY_SHELL` make variables (so GB3 can stub them); `make -n install` shows no sudo; `scripts/install.sh --dry-run` lists the staged files and includes every file under ui/.

### [ ] B20 · Docs (S)
- deps: B19
- files: README.md, AGENTS.md, CONTRIBUTING.md, FORGE_SPEC.md, .agents/README.md
- do: update the layout (C1), protocol summary (C3), demo mode (C5, including the `Sam` failure path and `Ctrl+Shift+D`), shortcuts (link docs/KEYS.md), dev loop (`make link`, `make install`, `make demo-reset`), test map (C9), and the old-data cleanup note (C2). Remove claims that Docker runs in `make test`. Point AGENTS.md at this file.
- accept: §0.1 D11 review table for every inventory doc is filled in the task note.
- verify: `make docs-check`

### [ ] B21 · QML import dir + Qt6 lint (S)  (formerly Q01)
- deps: GR
- files: scripts/qml-imports.sh, Makefile (`lint`)
- do: create `build/qml/qs/Commons` and `build/qml/qs/Ui` symlinks from `${OMARCHY_SHELL_DIR:-${OMARCHY_PATH:-/usr/share/omarchy}/shell}`; fail with a clear message if missing; use `/usr/lib/qt6/bin/qmllint` (F1).
- verify: `make lint`

### [ ] B22 · UI linter in code (M)  (formerly Q02)
- deps: GR
- files: tools/uilint/{package.json,package-lock.json,eslint.config.cjs,pragma-processor.cjs,rules.cjs,qml-rules.cjs,cli.cjs}, tools/uilint/test/*.test.cjs, tools/uilint/test/fixtures/, ui/.qmllint.ini, .gitignore (`tools/uilint/node_modules/`)
- do:
  1. Pin ESLint with an exact version in `package.json` (`"private": true`) and commit the lockfile.
  2. `pragma-processor.cjs` blanks `.pragma`/`.import` lines while keeping line numbers.
  3. `eslint.config.cjs` applies to `ui/lib/*.js` with the C10 JS limits (`complexity`, `max-depth`, `max-lines-per-function`, `max-params`, `max-lines`) and `no-restricted-syntax` for ES2015+ constructs. `Qt` is allowed as a global only in `Keymap.js`.
  4. `qml-rules.cjs` is a pure node module. It strips comments and strings with a small tokenizer, then checks:
     - the QML file length and the length, nesting and complexity of JS functions inside QML (C10)
     - UI placement rules, colour literals, `requestActivate`, relative imports and manifest entryPoints
     - the `.pragma library` first line, suppression syntax and the ≤ 5 suppression budget
  5. `cli.cjs` runs ESLint plus the QML rules over `ui/`. The `npm run lint` and `npm test` scripts use it.
  6. `ui/.qmllint.ini` raises qmllint's unqualified, unused-imports, missing-property and type categories to errors.
- accept: every QML rule and every ESLint limit has a passing and a failing fixture test; `npm --prefix tools/uilint run lint` exits 0 on the repo.
- verify: `npm --prefix tools/uilint ci && npm --prefix tools/uilint test && npm --prefix tools/uilint run lint`

### [ ] B23 · QML integration harness (L)  (formerly Q03)
- deps: B16, B17, B21
- files: tests/qml/run.sh, tests/qml/root/shell.qml, tests/qml/Harness.qml, tests/qml/scenarios/*.js
- do:
  1. `run.sh` builds the helper into a temp plugin tree staged exactly like `make install`; creates the root dir with Commons/Ui symlinks; sets `XDG_DATA_HOME` to a temp dir; runs `QT_QPA_PLATFORM=offscreen quickshell -p root` with a 60 s timeout. `shell.qml` instantiates Service (pointed at the staged tree, args `--demo --no-chatter --seed 1`), Panel with a mock shell facade (`serviceFor`, `hide` → `panel.close()`), and BarWidget with a mock bar.
  2. Each scenario logs `PASS name` / `FAIL name: reason`; finish with `Qt.exit(failures ? 1 : 0)`.
  3. Use `TestCase.keyClick` for keys (F2); find items by `objectName`.
- scenarios:
  - S1 service reaches `ready`, 3 accounts, 11 conversations.
  - S2 `j`,`j`,`Enter` opens the third conversation, the composer has focus, and unread for it becomes 0.
  - S3 typing `hello`, `Enter` → a new outgoing bubble that progresses to `delivered` (wait on events).
  - S4 `Escape` chain per C6 ends with `close()` called.
  - S5 Ctrl+K, type `ticket` → only Alex Chen is listed with a highlighted match; Escape clears.
  - S6 Ctrl+2 shows only Telegram; the rail shows Personal/Work under Telegram; the long title row elides.
  - S7 Ctrl+N, choose `Ben Okafor`, Enter → a new conversation opens.
  - S8 Sam: send → failed → `r` → delivered.
  - S9 BarWidget count equals the service's `unreadTotal` and updates after S2.
  - S10 close the Panel, recreate it → same `activeId` and draft text restored.
  - S11 for every `Actions.OWNERS` entry, the owning controller's `handles(action)` is true.
  - S12 at the minimum window size (`Style.space(760)×Style.space(540)`): rail, list and conversation panes all have width > 0 and don't overlap; the composer is visible; footer hints don't overflow.
  - S13 `RpcClient` with a fake transport: timeout fires the callback with an error after 15 s (fake timer); an error response maps to `Rpc.errorText`; an event line never resolves a pending request.
- rules: scenarios are independent (each starts from a fresh demo data dir, or resets via a new Service) and use no sleeps longer than needed to await an event (poll ≤ 5 s with a clear FAIL message).
- verify: `tests/qml/run.sh`

### [ ] GB · Phase B gate (M)
- deps: B01–B23 (including B15a)
- checks: all of §0.1 (D1–D11).
- plus:
  - GB1 `make test` passes (test-go with covergate, test-js with coverage gates, lint with omalint + uilint + Qt6 qmllint, docs-check, test-qml).
  - GB2 Harness stability: `for i in 1 2 3; do tests/qml/run.sh || exit 1; done`.
  - GB3 Sandbox install: `HOME=$(mktemp -d) OMARCHY=true OMARCHY_SHELL=true make install OPEN=0` exits 0. The staged plugin dir contains `manifest.json`, every file under `ui/`, `bin/oma-messenger-service` and `bin/dev/oma-messenger-service`, and contains no `backend/`, `vendor/` or `tests/`.
  - GB4 `omarchy plugin validate .` passes.
  - GB5 No scaffold remnants: `git grep -nE 'keyboard\.js|api\.token|43821|XMLHttpRequest|requestActivate' -- ui scripts tests Makefile` is empty.
  - GB6 Owner review (a human, not the implementing model). Run `make install` and check, at the default and minimum window size and in one light and one dark Omarchy theme:
    - long names elide; empty states (no search match; no chat open)
    - unread badges (including the selected row and muted chats)
    - compose: Enter sends, Shift+Enter adds a newline; Sam's failed → `r` retry
    - the full Escape chain; Ctrl+N new chat; Ctrl+1/2/0 and Ctrl+Tab
    - F1 help; bar widget count and click; Ctrl+Shift+D demo message and its notification

    The owner records feedback as new tasks B24+ (same format) and ticks GB only after those are done.
- note: record test counts and coverage tables.

### Phase Q — end-to-end, CI and mutation testing

### [ ] Q04 · Docker E2E update (M)
- deps: GB
- files: tests/e2e/Dockerfile, tests/e2e/run-in-container.sh, tests/e2e/shell.qml, scripts/test-docker.sh
- do: copy real Omarchy Commons/Ui into the build context (from `OMARCHY_SHELL_DIR`); use the real Service/Panel with `--demo --no-chatter`; keep the existing window/focus/workspace/close checks; add `wtype` flows `j`,`Enter`,`hello`,`Return` and assert through an `IpcHandler` `state()` returning JSON (`activeId`, last message text/status).
- verify: `make test-e2e`

### [x] Q05 · Coverage gate (S)
- note: superseded by R01e (`tools/covergate`). Nothing to do.

### [ ] Q06 · CI (M)
- deps: GB
- files: .github/workflows/ci.yml, .github/workflows/release.yml
- do:
  - `ci.yml` on PR and push: job `go-js` (ubuntu: setup-go 1.23, setup-node 22, `npm --prefix tools/uilint ci`, `make test-go test-js docs-check` and the omalint/uilint parts of `lint`); job `qml` (container `archlinux:base-devel`: install quickshell qt6-declarative go nodejs; clone `omacom/omarchy` at tag `v4.0.4`; `OMARCHY_SHELL_DIR=…/shell make lint test-qml`).
  - `release.yml`: publish only after `ci.yml` succeeds (use `workflow_run` or `needs`); build binaries on `v*` tags and on main pushes that touch `backend/**`.

### [ ] Q07 · Mutation testing (M)
- deps: GB
- files: scripts/mutation.sh, Makefile (`mutation` target, not part of `make test`)
- do:
  1. Run `gremlins` via `go run github.com/go-gremlins/gremlins/cmd/gremlins@<version>`. Pin the latest release tag at implementation time in the script. It needs network once; it is not run offline.
  2. Run it on `backend/internal/{domain,app/policy,store,app,api}` with its efficacy-threshold flag (check `gremlins unleash --help`).
  3. Thresholds: efficacy ≥ 90 % for `domain` and `app/policy`; ≥ 80 % for `store`, `app`, `api`.
- accept: `make mutation` passes. Every surviving mutant in the report is either killed by a new test or listed in the task note with a one-line reason it's equivalent.
- verify: `make mutation`

### [ ] GQ · Phase Q gate (S)
- deps: Q04, Q06, Q07
- checks: all of §0.1 (D1–D11).
- plus:
  - GQ1 `make test-e2e` passes twice in a row.
  - GQ2 CI is green on a pull request for both jobs (`go-js`, `qml`). Put the run URL in the note.
  - GQ3 CI actually gates: open a throwaway PR that breaks one assertion, confirm CI goes red, then close it. Put the URL in the note.
  - GQ4 `make mutation` passes the Q07 thresholds.
  - GQ5 The release workflow only publishes after CI succeeds (inspect `needs`/`workflow_run` in `release.yml`).

### Phase D — feature complete (real services)

Do D only after GQ. Each connector task needs golden tests built from recorded/constructed library structs (`testdata/*.golden`, regenerated with `-update`), a `Fuzz<Normalize…>` target per normalizer, and a `conformance_test.go` (D00). Never hit real services in tests.

### [ ] D00 · Connector conformance suite (M)
- deps: GQ
- files: backend/internal/connector/connectortest/conformance.go, backend/internal/connector/demo/conformance_test.go, C10 row update in this file
- do:
  1. `Run(t *testing.T, h Harness)`, where `Harness` provides `New(t) connector.Connector` plus hooks to simulate the remote side: `Deliver(convRemoteID, text)`, `Ack(remoteID, status)`, `Fail(next bool)`.
  2. Shared checks:
     - `Run` reports `connecting` then `connected`; ctx cancel returns within 1 s.
     - `Send` returns quickly and its progress arrives via `OutgoingStatus` in forward order only.
     - Incoming messages carry RemoteID, SenderName and Created.
     - Duplicate deliveries reuse the RemoteID.
     - `MarkRead` doesn't error on an empty conversation.
     - No goroutine survives ctx cancel.
  3. Add a C10 row: `connector/connectortest` may import connector, domain, connector/clocktest.
- accept: the demo connector passes; every later connector (D03, D07) must add a `conformance_test.go` that calls `connectortest.Run`.
- verify: `go test -mod=vendor -race ./backend/internal/connector/...`

### [ ] D01 · Auth protocol (M)
- files: C3 table (this file), backend/internal/app, backend/internal/rpc, ui/lib/Rpc.js
- do: add methods `accounts.add{service,name}` → Account (`needs-auth`), `auth.submit{accountId, step, value}` (steps `phone`, `code`, `password`), `accounts.logout{accountId}`, `accounts.remove{accountId}` (cascade delete + session files). Add events `auth.qr{accountId, png:base64, expiresAt}`, `auth.step{accountId, step, hint}`, `auth.done{accountId}`, `auth.failed{accountId, message}`. Extend the Connector with an optional `Authenticator` interface.

### [ ] D02 · Account setup UI (M)
- deps: D01
- files: ui/components/AccountSetup.qml, ui/Panel.qml, ui/lib/Keymap.js
- do: rail `+` menu → "Add WhatsApp account…" / "Add Telegram account…"; QR shown with `Image { source: "data:image/png;base64," + png }` and a countdown; phone/code/password fields; context `dialog`.

### [ ] D03 · WhatsApp connect + pairing (L)
- deps: D01
- files: backend/internal/connector/whatsapp/*
- do: `go.mau.fi/whatsmeow` with `sqlstore` on `<data-dir>/whatsapp/<accountId>.db` (driver `sqlite`); QR pairing (render the PNG with `github.com/skip2/go-qrcode`) and pair-code login; reconnect handled by whatsmeow; status mapping to C4. Run `go mod vendor`.

### [ ] D04 · WhatsApp sync (L)
- deps: D03
- do: HistorySync → conversations/messages (dedupe by message ID); contacts/push names → `Contact`; group info → title/members; normalize LID vs PN JIDs to one RemoteID per chat. Golden tests from constructed `events.HistorySync`.

### [ ] D05 · WhatsApp live receive (M)
- deps: D04
- do: `events.Message` (conversation/extended text only for now) → `Sink.Incoming`; `events.Receipt` → `OutgoingStatus`; `events.ChatPresence` → `Typing`. Golden tests.

### [ ] D06 · WhatsApp send + read (M)
- deps: D05
- do: `SendMessage` with text; map the response ID to `OutgoingStatus(sent)`; `MarkRead` sends read receipts for unread incoming IDs.

### [ ] D07 · Telegram connect + login (L)
- deps: D01
- files: backend/internal/connector/telegram/*
- do: `github.com/gotd/td`; settings `telegramApiId`/`telegramApiHash` (README explains my.telegram.org); session storage `<data-dir>/telegram/<accountId>.session` (0600); phone → code → 2FA password flow via `auth.Flow`; QR login optional. Tests use `tgtest` or interface fakes.

### [ ] D08 · Telegram dialogs sync (L)
- deps: D07
- do: store migration 2: `peers(account_id, remote_id, access_hash, type)`; paginate `messages.getDialogs`; fetch the last 50 messages per dialog lazily on open via the optional `connector.HistoryLoader` interface (C4); `Commands` calls it when `messages.list` runs out of stored history. Golden tests.

### [ ] D09 · Telegram updates (L)
- deps: D08
- do: `updates.Manager` for gap recovery; new messages → Incoming; read outbox → `OutgoingStatus(read)`; typing.

### [ ] D10 · Telegram send + read (M)
- deps: D09
- do: `messages.sendMessage` with a random ID; map to the server ID; `messages.readHistory` on MarkRead.

### [ ] D11 · Edits and deletes (M)
- do: migration adds `edited_at`, `deleted`; the optional `connector.EditSink` interface (C4), implemented by `app.Ingest`; events `message.updated`; UI shows "edited" and "This message was deleted".

### [ ] D12 · Replies/quotes (M)
- do: migration adds `reply_to_remote_id`; quoted snippet in the bubble; `R` in conversation context replies to the newest incoming message, or to the message under the cursor once message selection exists.

### [ ] D13 · Reactions (M)
- do: the optional `connector.ReactionSink` interface (C4), implemented by `app.Ingest`; table `reactions(message_id, sender_id, emoji)`; chips under the bubble; `+` key opens an emoji row.

### [ ] D14 · Media receive (L)
- do: `attachments(message_id, kind, mime, name, size, local_path, thumb)`; download into `<data-dir>/media/`; inline image thumbnails; `o` opens the attachment with `xdg-open`.

### [ ] D15 · Media send (M)
- do: `Ctrl+O` path prompt with completion and drag-and-drop onto the conversation; size limits per service.

### [ ] D16 · Full-text search (M)
- do: migration creates an FTS5 table `messages_fts`, synced by triggers; `search.messages{query}` → hits with conversation and snippet; search results show a "Messages" section; Enter jumps to the message (`messages.around{messageId}`).

### [ ] D17 · Pin and archive (S)
- do: columns `pinned`, `archived`; pinned sort first; archived hidden behind an "Archived" rail item; keys `p` and `A`.

### [ ] D18 · Notification click opens the chat (S)
- do: `notify-send --action=open=Open --wait` in a goroutine; on `open`, exec `omarchy-shell shell summon io.github.omamessenger '{"conversationId":"…"}'`.

### [ ] D19 · Demo off when real accounts exist (S)
- do: setting `demoMode` (default true until the first real account is added); Service.qml starts the helper without `--demo` when false.

### [ ] D20 · Release 0.3.0 (S)
- do: README privacy section (local-only data, WhatsApp ban risk for unofficial clients, Telegram API credentials); `make build-all`; tag `v0.3.0`.

### [ ] GD · Phase D gate — feature complete (M)
- deps: D00–D20
- checks: all of §0.1 (D1–D11).
- plus:
  - GD1 Every normalizer has golden tests; `go test ./backend/... -run Golden -update` produces no diff.
  - GD2 Every `Fuzz*` target runs 60 s without failure.
  - GD3 Offline: `docker run --rm --network none -v "$PWD":/src -w /src golang:1.23 go test -mod=vendor ./backend/...` passes.
  - GD4 The WhatsApp and Telegram connectors pass `connectortest.Run` against their fakes.
  - GD5 Privacy: an integration test runs the helper with fake WhatsApp/Telegram sessions and asserts that stderr contains no message text, names, phone numbers, QR payloads or session data. Session files are 0600 and the data dir is 0700 (tested).
  - GD6 Every `FORGE_SPEC.md` "MVP acceptance" item is ticked, with the test or check that proves it named next to it.
  - GD7 Owner live verification (human), results recorded in the note:
    - pair WhatsApp; log in to Telegram with 2FA
    - receive, send, read receipts both ways; edits/deletes, replies, reactions; media in and out; search jump
    - restart persistence; offline → online reconnect; notification click opens the chat
    - two accounts on one service; logout/remove wipes the data
  - GD8 `make test`, `make test-e2e` and `make mutation` all pass on the release commit; CI is green; tag created (D20).
