# Decisions

Design choices that are not obvious from the code, and why they were made. Read the relevant entry before reversing one; if you do reverse it, update the entry.

## JSON-RPC over the helper's stdin and stdout

The UI and helper talk JSON-RPC 2.0, one object per line, over the helper's stdin and stdout. The alternative was a Unix socket.

- Quickshell's `Process` gives the UI the helper's stdio for free, and Omarchy's shell already owns the process. With stdio there is no socket file to place, protect or clean up, no address to discover, and no other local process can connect.
- A local pipe is as fast as a local socket. Traffic is a few messages a second at human pace, so neither would be the bottleneck.
- The helper lives exactly as long as the UI's process handle: when the shell stops it, stdin closes and the helper exits.

## sourcegraph/jsonrpc2 instead of our own framing

The helper uses `github.com/sourcegraph/jsonrpc2` rather than hand-written line framing and dispatch. A maintained library handles request ids, concurrent replies and notifications, and the UI speaks a documented standard.

Trade-off: a line that is not valid JSON closes the connection. The UI only sends `JSON.stringify` output, and the UI's service restarts the helper if it exits.

## A failed send is a message, not an error

`messages.send` returns the stored message with status `failed` when the service refuses it, instead of an error. The UI already shows failed messages with a retry action, so an error would report the same thing twice.

## The app layer uses the concrete store

`app` takes `*store.Store` rather than an interface. There is one store, and tests use a real SQLite database in a temporary directory, so an interface would only mirror its methods. Interfaces are used where there are real alternatives: connectors, desktop notifications and the UI connection.

## Real time in code, synctest in tests

There is no clock interface. Code uses the `time` package, and tests of timing (connector restarts, the fake connectors' delays) run inside `testing/synctest`, where time advances instantly and deterministically.

## Release binaries, installed automatically on first open

Helper binaries are published on GitHub Releases with `SHA256SUMS` and build provenance, never committed. `helper-version` names the release, and `scripts/install-helper.sh` downloads and verifies it.

`omarchy plugin add` only clones the repository; by Omarchy's design a plugin has no install hook to run afterwards. Asking the user to then run a script or press a separate "Install helper" button made OmaMessenger a two-step install when every other plugin is one — Omarchy's own Spotify plugin resolves the same gap the same way. So the window now runs the installer itself, the first time it opens and finds the helper missing or pinned to a different version than `helper-version` (a plugin update changes that file, and the launcher looks for a binary named after the new version, so it is "missing" again until the next open installs it). It shows a quiet "Installing the helper…" state and starts the helper on success. A failed install (offline, a bad checksum, no release yet) shows the error with a Retry button, the only remaining manual path; it does not retry on its own on the next open, and it never touches a working helper already in place, since the installer verifies into a version-named temporary file and only `mv`s it over the version-named destination after that check passes.

Verification — the release's `SHA256SUMS` and the binary's own reported version — stays mandatory either way. Loading the plugin itself still never downloads anything: the trigger is opening the window, never the shell starting. A local `make build` in `bin/dev/` still takes precedence, for development.

## No vendor directory

Go dependencies come from the module cache, pinned by `go.sum`. Committing `vendor/` added about 1,800 files to every review for little benefit.

## Existing tools before custom ones

Lint and coverage rules are configuration for golangci-lint (`.golangci.yml`) and go-test-coverage (`.testcoverage.yml`). The only custom check is `tools/nologcontent`, which keeps message content out of logs, because no existing tool knows which values are private.

## No compositor tests

Automated tests do not start Hyprland or Docker. They were slow and caught little that unit, offscreen QML and helper tests did not. Visual and window-manager behaviour is checked by hand in the installed plugin.

## A typed adapter for Omarchy's theme tokens

Omarchy declares nested token groups (`Style.font`, `Color.popups`, …) as plain `QtObject` properties, so Qt 6 qmllint reports every use as a missing property. `ui/theme/Theme.qml` re-exports them with types, which keeps lint at zero warnings and still catches typos. It is the only file allowed to suppress that warning.

## Fake accounts only in test builds

The scripted fake connectors that tests run against are compiled in only with the `fake` build tag (`make build-fake`). Release builds cannot show seeded data, so it can never mix with real messages, and the product has no demo mode to explain.

## Providers behind one interface

Adding an account used to mean `backend/accounts.go` and `app.AddAccount` both knowing Telegram's rules: its credential shape, its app keys, its id prefix. `connector.Provider` moves all of that behind one interface (`Service`, `Name`, `Prepare`, `Connect`, `Forget`) that each service's own package implements. The registry becomes a map of providers, keyed by service id, and `app.AddAccount` only maps a provider's `ErrInvalidSetup` or an unknown service to invalid input; it no longer validates API keys itself.

Prepare takes a plain `map[string]string` rather than a typed options struct, because each service's setup is different (Telegram's API id and hash today, something else tomorrow) and the protocol already carries them as JSON strings. The cost is that a provider must parse its own options and return `ErrInvalidSetup` with a safe message when they are wrong; that cost sits in the one package that knows what valid means for that service, which is the same trade `Provider` makes everywhere else.

`accounts.add`'s `apiId`/`apiHash` fields stay as they were, merged into `options` by the server before they reach `app.NewAccount`, so no existing UI build and no saved Telegram account's `tg-` id prefix breaks. New accounts get an id prefixed with their service name instead, since the registry no longer special-cases Telegram's.

## Clicking a notification: notify-send's "default" action, not D-Bus

A message notification carries its conversation id so clicking it can open that conversation. `notify-send -A default=Open` registers the freedesktop "default" action and blocks (`--wait` is implied) until the notification closes or an action is chosen, printing the chosen action's name to stdout. Omarchy's own notification daemon (`/usr/share/omarchy/shell/plugins/notifications/Service.qml`, `invokePopupDefault`) already treats a live notification's `default`-identified action as "clicked the body", falling back to focusing the sending app's window only when no such action is registered — so this needed no change on Omarchy's side.

The alternative was a D-Bus session connection from the helper, watching `org.freedesktop.Notifications.ActionInvoked` directly. notify-send's stdout is simpler: it is already the helper's only way to show a notification, needs no new dependency, and keeps the daemon itself unaware that OmaMessenger exists. The cost is one goroutine per notification, blocked in notify-send until it closes; `notify.Desktop` bounds that with a timeout so a notification that is somehow never closed cannot leak it.

## Sending photos and files

`domain.Media` gained a `Path` field, tagged `json:"-"`, rather than a separate type the server and connectors pass alongside a message. A dedicated field (as the brief suggested as an alternative) would mean threading a second value through `Commands.Send`, `Retry` and every `Dispatcher`, when the local file is really one more thing media about an outgoing message; `json:"-"` keeps it out of the wire protocol (the UI already knows the path it offered; a remote message has none) without adding a parameter everywhere Media already travels.

The outgoing media area (`cache.Outgoing`) names a stored file by the message's id and its original file name (`<id>-<name>`), set before the message is first stored by generating the id in `app` rather than letting the store assign one. `domain.Media.Path` cannot survive a reload from the store (it is deliberately excluded from the JSON column), so `Retry`, which re-reads the message from the store, recomputes the same path from the id and file name it does keep, instead of needing Path to persist.

A captionless attachment reuses the existing placeholder convention for media (`domain.MediaPlaceholder`, the same `"[Photo]"`/`"[Video]"`/`"[File]"` labels a connector already gives incoming media with no caption) rather than allowing an empty stored message. The UI already hides a caption that exactly matches its media's label, so this needed no UI change; the Telegram connector strips the placeholder back to an empty caption before sending, since it is a local display convention, not real text to deliver.

The file chooser is `QtQuick.Dialogs.FileDialog`, used directly from the composer. The plugin's manifest only offers `service`, `panel` and `bar` entry points, not a new Omarchy overlay like `image-picker` (which is also scoped to one fixed directory of images, not an arbitrary file), and `xdg-desktop-portal` is already running on the test machine with a Hyprland and a GTK backend registered; loading `FileDialog` under the offscreen platform showed Qt routing its `open()` straight at that portal (it only failed here on the portal's own sandbox permission check, not on anything Quickshell-specific). `FileDialog` needed no new helper method, so this is the simpler of the two options the brief allowed for.

Pasting an image goes through a new `media.paste` method instead of teaching the UI to read the clipboard, because Qt's QML has no cross-process way to read Wayland clipboard data (only `wl-paste`, a separate process, does); the helper already shells out to external processes for notify-send, so one more for `wl-paste` keeps that logic out of the UI. The composer always intercepts Ctrl+V while composing and asks the helper first; when the clipboard has no image, the composer is told to fall back to a plain native paste, rather than the key router trying to guess the clipboard's contents before deciding whether to intercept the key at all.

## Reactions: a Sink method of their own, one reaction per person, custom emoji skipped

Telegram reports a reaction change two ways: a full `UpdateEditMessage`, which carries the message's new text, media and reactions together, and `UpdateMessageReactions`, which carries only the peer, message id and the new reaction counts. The first already fits `Sink.Edited`, since `EditMessage` was going to need a `reactions` argument anyway. The second does not: `Edited`'s contract replaces a message's text and media unconditionally, so feeding it empty values from a reactions-only update would blank out a message's real content. Fetching the full message first to avoid that would cost a network round trip for every reaction click, just to reuse one method. `Sink` already accepts this kind of growth (`Organized` was added the same way for pin and archive sync), so a second method, `Reacted`, carries just the reaction chips, and the store gained `SetReactions` to match: it touches only the `reactions` column.

The UI sends one emoji per `messages.react` call, not a set, and `Reaction` has no way to say "pick several". Telegram's API allows more, but `messages.sendReaction` takes whatever list is given, so sending a single reaction already replaces any previous one from the same person; a second reaction would need its own chip, its own click target and its own place in the picker for a case most chat UIs do not offer either. `Reactions.emojiToSend` (`ui/lib/Reactions.js`) encodes the one-reaction choice: clicking a chip that is already the user's own clears it, and any other pick replaces it.

A custom emoji reaction (`tg.ReactionCustomEmoji`, a sticker rather than a character) is left out of `reactions` entirely rather than shown as a generic placeholder. Telegram's own clients render the actual sticker, which this UI has no way to draw; a placeholder chip would look like a real reaction with the wrong picture, which is worse than one chip fewer.

## WhatsApp's session database uses the same pure-Go SQLite driver as the message store

whatsmeow's `sqlstore.Container` normally opens its own `*sql.DB` by calling `sql.Open` with a dialect string that doubles as the database/sql driver name, which is how its own examples register `mattn/go-sqlite3` (cgo) or `sqlite3-fk-wal`. The helper is built with `CGO_ENABLED=0`, so that path is out. Reading `go.mau.fi/util/dbutil`'s source showed the fix needs no library change: `ParseDialect` only checks that the string has a `sqlite` prefix to pick its SQL quirks, and `sqlstore.NewWithDB` takes an already-open `*sql.DB` and that same string separately, never calling `sql.Open` itself. So the connector opens its own connection exactly as `store.Open` does, with `modernc.org/sqlite`'s pure-Go driver registered under the name `"sqlite"`, and hands both the `*sql.DB` and the string `"sqlite"` to `sqlstore.NewWithDB`; dbutil accepts it as a `sqlite` dialect and generates the right SQL, with no cgo anywhere in the build.

## WhatsApp pairing wraps whatsmeow's client behind a small `device` interface

Unlike gotd/td, which already exposes its lowest-level RPC transport as the `tg.Invoker` interface Telegram's own tests fake, `*whatsmeow.Client` has no such seam: `Connect`, `GetQRChannel`, `PairPhone` and the rest are concrete methods with no public way to substitute a fake network underneath them. `backend/internal/connector/whatsapp/device.go` defines `device`, an interface over exactly the whatsmeow calls the connector and its pairing sequence need (connect, disconnect, close the session database, check whether it is already paired, log out, start a pairing attempt and watch its QR codes, pair by phone, and watch for connected/disconnected/stopped status) so `Connector.Run`, `pair` and their tests drive a hand-written fake instead of ever reaching WhatsApp's servers; `waDevice` is the one production implementation, adapting a real client to it. This is the same shape `connector.Connector` itself and Telegram's `authAPI` already use.

QR and phone pairing share the one `device.qrCodes` channel rather than two separate interface methods, because that is how whatsmeow itself works: `GetQRChannel` keeps running and reports the same final "success" or "error" event no matter which way pairing actually finished, so `pair`'s loop just also watches for a phone number answer on the side and calls `pairPhone` when one arrives, instead of juggling two completion signals for one outcome.

## WhatsApp's account removal logs out the connected device, best effort, then deletes the local session

`connector.Provider.Forget(dir, accountID string) error` still carries no context and no live connection, so it only ever deletes local files, exactly as Telegram's `Forget` does. But a WhatsApp connector that is still running when the user removes it has a live connection worth using: `connector.LogoutOnRemove` is a capability interface with one method, `Logout(ctx) error`, that `Manager.Remove` type-asserts on the connector it is about to stop. When the running connector implements it, `Manager.Remove` calls `Logout` with a bounded `context.WithTimeout` before cancelling the connector's own context, since cancelling first would close the connection `Logout` needs; the result is ignored (commented at the call site), because the local session is deleted either way. WhatsApp's `Connector` tracks the `device` its `Run` is currently using in a mutex-guarded field and implements `Logout` by calling the device's `logOut`. This is why the capability is a new optional interface rather than a change to `Forget`'s signature or to `Connector`: `connector.Connector` and `connector.Sink` stay the stable, published boundary, and a connector that cannot log out (or is not running) just does not satisfy it.

Telegram has no such connection to use here and does not implement `LogoutOnRemove`; its account's session lapses from the server's own view once it stops being used, the same as before.

## WhatsApp's media references live in their own database, not whatsmeow's session

A message's photo, video or file carries a `mediaRef` (direct path, media key, file hashes, length and mimetype) that a later wave's `MediaFetcher` will need to download it, since WhatsApp's end-to-end messages have no server-side copy to re-fetch a reference from the way Telegram's do. This connector saves that reference as soon as a message arrives, in its own SQLite database beside the account's whatsmeow session (`<account>-media.db`, mode 0600, like the session itself) rather than as an extra table inside that session's database. whatsmeow's `sqlstore.Container` owns that schema and upgrades it on its own terms; adding a foreign table to it would work today but ties this connector's data to whatever whatsmeow's own migrations do next. `Provider.Forget` deletes this file alongside the session when an account is removed.

## WhatsApp's chat organizing and naming caches live in memory for the process's lifetime

Pin and archive events from the phone (`events.Pin`, `events.Archive`) each report only the one flag that changed, but `Sink.Organized` takes both, so the connector keeps a `remote id -> {pinned, archived}` cache, seeded from history sync and updated by each event, and always reports the merged pair. Reaction chips work the same way: WhatsApp reports one person's reaction change at a time, never a conversation's full tally, so a `remote id -> {sender -> emoji}` cache is kept and recomputed into the chip list `Sink.Reacted` expects on every change. Group and contact names resolved to fill in a blank title (see below) are cached too, so a group is never asked for its name twice.

All three caches are fields on `Connector`, surviving a reconnect within the same helper process, but starting empty again after the helper itself restarts, same as Telegram's own `remotes`/`sent` caches. A live organizing or reaction event arriving before anything has repopulated the cache falls back to the one flag it actually carries (the other defaults to false) or reports a fresh tally of just itself; a subsequent history sync corrects it. This was accepted rather than built out further because the store's own copy of pinned, archived and reactions is the durable one, and a transient, self-correcting gap right after a restart was judged cheaper than giving this connector a way to read state back out of the store it only ever writes to.

## WhatsApp's unnamed conversations fall back to a resolved or generic name, never to no name at all

`store.EnsureConversation` drops a conversation with no title rather than creating or updating one, since the UI has nowhere to show a nameless chat. History sync does not reliably carry every conversation's display name: a direct chat's name comes from a separate push name list in the same sync blob (processed first, so it is cached before the conversation list needs it), and a group's name is sometimes missing and has to be fetched with `GetGroupInfo`, bounded by a short timeout and cached so a slow or unreachable group is asked for once. When even that resolves to nothing, a group falls back to the literal title "Group" and a direct chat falls back to its phone number (`"+" + the JID's user part`), the way WhatsApp's own clients show an unsaved contact. The same resolution runs for a live message's chat when it arrives before any sync has reached it.

## WhatsApp's older history on scroll is not implemented

`connector.HistoryLoader.LoadOlder` is left unimplemented for WhatsApp in this wave: scrolling past what history sync already delivered loads nothing more, the same as a connector with no `HistoryLoader` at all. whatsmeow exposes `BuildHistorySyncRequest`, but it is not a request/response call — it builds a protocol message that has to be sent to the user's own primary device, which replies, asynchronously and without a correlating id, with another `events.HistorySync` of type `ON_DEMAND` whenever the phone gets to it. Matching that reply back to the specific `LoadOlder` call that asked for it, across however many are in flight, is not a seam `whatsmeow` gives a caller; building one would mean guessing at protocol behaviour this package cannot verify without a live account, which the test suite cannot reach. History sync's own bootstrap already delivers a worthwhile amount of recent history per conversation, so this was judged not worth that risk for a first wave. The README notes the limit.

## WhatsApp's mute state from the phone is not synced

`events.Mute` is received and intentionally dropped rather than reported anywhere: `connector.Sink` has no method for it, by design. `Sink.Organized` carries pinned and archived because `store.SetOrganized` keeps both in step with the service; the store's `muted` column has no equivalent sync path at all; `store.refreshConversation`, which `EnsureConversation` uses to update an existing conversation, deliberately never touches it, and the only way to set it is the UI's own local `conversations.setMuted` command. Muted is, by this app's existing design, a local preference rather than a synced one, the same as `hidden`. Adding a new `Sink` method just for WhatsApp's mute changes would be a bigger change than this one connector's wave justifies, given `connector.Sink` is meant to stay stable once a real connector ships; if muting from the phone turns out to matter, it is a Sink change to make deliberately, with Telegram's own mute considered at the same time, not a one-off for WhatsApp.
