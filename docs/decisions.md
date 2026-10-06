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

There is no clock interface. Code uses the `time` package, and tests of timing (connector restarts, demo delays, chatter) run inside `testing/synctest`, where time advances instantly and deterministically.

## Release binaries, installed on request

Helper binaries are published on GitHub Releases with `SHA256SUMS` and build provenance, never committed. `helper-version` names the release, and `scripts/install-helper.sh` downloads and verifies it only when the user asks. Loading the plugin never downloads anything. A local `make build` in `bin/dev/` takes precedence, for development.

## No vendor directory

Go dependencies come from the module cache, pinned by `go.sum`. Committing `vendor/` added about 1,800 files to every review for little benefit.

## Existing tools before custom ones

Lint and coverage rules are configuration for golangci-lint (`.golangci.yml`) and go-test-coverage (`.testcoverage.yml`). The only custom check is `tools/nologcontent`, which keeps message content out of logs, because no existing tool knows which values are private.

## No compositor tests

Automated tests do not start Hyprland or Docker. They were slow and caught little that unit, offscreen QML and helper tests did not. Visual and window-manager behaviour is checked by hand in the installed plugin.

## A typed adapter for Omarchy's theme tokens

Omarchy declares nested token groups (`Style.font`, `Color.popups`, …) as plain `QtObject` properties, so Qt 6 qmllint reports every use as a missing property. `ui/theme/Theme.qml` re-exports them with types, which keeps lint at zero warnings and still catches typos. It is the only file allowed to suppress that warning.

## The demo has its own database

`--demo` uses `demo.db`, never `messages.db`, so seeded data can never mix with real messages.
