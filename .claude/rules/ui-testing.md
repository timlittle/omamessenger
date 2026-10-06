## UI Testing

Two layers, both headless. Never test against the developer's running Hyprland session or `omarchy-shell`.

### Logic: node

- Everything that can be a pure function lives in `ui/lib` and is tested with node (see javascript.md). This is where most UI tests belong

### Components: offscreen Quickshell

- `qmltestrunner` cannot load our UI because `qs.Commons` depends on types compiled into the `quickshell` binary. Run QML tests with `QT_QPA_PLATFORM=offscreen quickshell -p <root>` instead
- `make test-qml` runs every `tests/qml/<Component>/shell.qml` in its own root under `build/qml-tests/`, which also links the `ui/`, `bin/` and `scripts/` trees and Omarchy's `Commons` and `Ui`. A test imports `"ui/components"` and `qs.Commons` as usual. `XDG_DATA_HOME` points inside that root, so nothing touches your real data
- A test is a `ShellRoot` that builds the component with fake data, drives it, checks values (not just that properties exist), logs `PASS <name>` and calls `Qt.exit(0)`, or logs `FAIL <reason>` with `console.error` and calls `Qt.exit(1)`
- Start the checks from a `Timer { running: true; interval: 0 }`, not `Component.onCompleted`: Quickshell ignores `Qt.exit()` until it has finished loading
- Put the component in a `FloatingWindow` when it needs a size or focus
- Drive real key events with `import QtTest` and `TestCase { id: t; when: false }`, then `t.keyClick(Qt.Key_Down)`
- Components get fake data through their properties; no helper process is needed. Tests for `ui/service` start the helper with `--demo`
- Every test has run once with a deliberately broken expectation, to see it fail
- No compositor or Docker-based end-to-end tests: they were slow and caught little. Manual checks happen in the installed plugin (`make install-local`)
