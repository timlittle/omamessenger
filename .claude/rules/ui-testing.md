## UI Testing

Two layers, both headless. Never test against the developer's running Hyprland session or `omarchy-shell`.

### Logic: node

- Everything that can be a pure function lives in `ui/lib` and is tested with node (see javascript.md). This is where most UI tests belong

### Components: offscreen Quickshell

- `qmltestrunner` cannot load our UI because `qs.Commons` depends on types compiled into the `quickshell` binary. Run QML tests with `QT_QPA_PLATFORM=offscreen quickshell -p <root>` instead
- `make test-qml` unsets `WAYLAND_DISPLAY` and `HYPRLAND_INSTANCE_SIGNATURE` and points `DBUS_SESSION_BUS_ADDRESS` nowhere (as `make test-go` does), so no test reaches the running desktop or shows a notification on it. It runs every `tests/qml/<Component>/shell.qml` in its own root under `build/qml-tests/`, which also links the `ui/`, `bin/` and `scripts/` trees and Omarchy's `Commons` and `Ui`. A test imports `"ui/components"` and `qs.Commons` as usual. `XDG_DATA_HOME` points inside that root, so nothing touches your real data. Each run also gets a throwaway `XDG_RUNTIME_DIR`, deleted afterwards: quickshell leaves a log folder per instance in the runtime directory, and filling the real one (a small tmpfs) has crashed Hyprland. Anything new that starts quickshell must do the same
- A test is a `ShellRoot` that builds the component with fake data, drives it, checks values (not just that properties exist), logs `PASS <name>` and calls `Qt.exit(0)`, or logs `FAIL <reason>` with `console.error` and calls `Qt.exit(1)`. `tests/qml/Check.js` (`import "Check.js" as Check`) provides `Check.fail`, `Check.find` and `Check.texts`; use them rather than writing your own
- Start the checks from a `Timer { running: true; interval: 0 }`, not `Component.onCompleted`: Quickshell ignores `Qt.exit()` until it has finished loading
- A test that polls for asynchronous helper state (almost all of them) uses `tests/qml/Stepper.qml` (`make test-qml` links it into every root, the same way it links `Check.js`) rather than writing its own steps array, attempts counter and timers. Set `steps` and `startFn` and let it run them in order, or leave `steps` empty and drive your own chain of named check functions with `stepper.retry(fn)` instead — see `tests/qml/Flows` and `tests/qml/ChatNavigation` for the two shapes. A test that drives a real `Panel` without a real host uses `tests/qml/FakeShell.qml` for the `shell` property, unless it needs a variant (`tests/qml/Flows` tracks every `hide` call; `tests/qml/BarWidget` has no panel at all)
- Put the component in a `FloatingWindow` when it needs a size or focus
- Drive real key events with `import QtTest` and `TestCase { id: t; when: false }`, then `t.keyClick(Qt.Key_Down)`
- Components get fake data through their properties; no helper process is needed. Tests that need a running helper get the test build (`make build-fake`), whose fake accounts give them data
- A test directory holding a `no-dev-build` file gets the launcher without `bin/dev`, so the helper starts out not installed; `OMA_RELEASE_BASE` points the installer at `release/` in the test root
- Every test has run once with a deliberately broken expectation, to see it fail
- No compositor or Docker-based end-to-end tests: they were slow and caught little. Manual checks happen in the installed plugin (`make install-local`)

### Which layer owns a test

One behaviour, one owning layer, plus one end-to-end smoke pass. Put a check at the layer that can fail for that reason and no other, then stop: a higher layer only proves wiring (its keystroke or command reached the right controller, with the right arguments) or reaches the outcome once, as a smoke check, never the outcome's own detail.

- A pure decision (what counts as unread, how a key resolves to an action, how commands rank against a query) belongs to its `ui/lib` function and is proven exhaustively there (`Rail.js`, `Keymap.js`, `Palette.js`, …)
- A protocol-level outcome (a send reaching delivered, a retry, what a connector reports) belongs to `backend/` Go tests (`app`, `connector`, `server`) and is proven exhaustively there
- A controller test (`tests/qml/Controllers`, `tests/qml/ListController`, …) wires a `ui/lib` decision or a helper round trip into reactive QML state. It checks the wiring — the right argument reached the right call, the right property changed — not the decision's own outcome in detail: that is already covered above
- A component test (`tests/qml/ConversationList`, `tests/qml/Composer`, …) checks rendering and user intent (a click or key producing the right signal) given fixed input properties. It does not recompute whether that input was the right one to show; that is the controller's and the `ui/lib` function's job
- `tests/qml/Flows` is the one end-to-end smoke suite: it drives real user journeys with real key events against the fake helper and checks each reaches its outcome (opened, sent and delivered once, filtered, closed, …), not the detail behind it. Something belongs there only when it is the sole place real keyboard delivery, a real window or a real helper process can be exercised at all (for example: the close question, quitting the helper, the minimum window size) — not because it is convenient to re-check an outcome one more time
- Before adding an assertion, ask whether a lower layer already proves it; if so, assert only that the keystroke or command reached the right place, not what it did once it got there
