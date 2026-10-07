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

## WhatsApp's account removal deletes the local session only

Telegram's `Forget` already only deletes local files; it never calls out to Telegram to unlink the session, since `connector.Provider.Forget(dir, accountID string) error` carries no context and no live connection to use. WhatsApp's `Forget` follows the same shape for the same reason, deleting the account's session database and the extra `-wal`/`-shm` files WAL mode leaves beside it. A live "unlink this device" call to WhatsApp would need a fresh connection opened inside `Forget` with no context to bound it or cancel it, and no way to test it without reaching WhatsApp's real servers; the account's phone number view of the delinked device lapses on its own once the session stops reconnecting; a tidier server-side unlink is no longer a wave-one problem.
