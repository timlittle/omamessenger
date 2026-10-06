## UI Testing

Two layers, both headless. Never test against the developer's running Hyprland session or `omarchy-shell`.

### Logic: node

- Everything that can be a pure function lives in `ui/lib` and is tested with node (see javascript.md). This is where most UI tests belong

### Components: offscreen Quickshell

- `qmltestrunner` cannot load our UI because `qs.Commons` depends on types compiled into the `quickshell` binary. Run QML tests with `QT_QPA_PLATFORM=offscreen quickshell -p <root>` instead
- `<root>` holds `Commons` and `Ui` symlinks to Omarchy's shell (`scripts/qml-imports.sh` creates them), so `qs.*` imports resolve
- A test is a `shell.qml` that builds the component with fake data, drives it, checks the result and ends with `Qt.exit(0)` on success or `Qt.exit(1)` with a message on stderr
- Drive real key events with `import QtTest` and `TestCase { id: t; when: false }`, then `t.keyClick(Qt.Key_Down)`
- Components get fake data through their properties; no helper process is needed. Tests for `ui/service` start the helper with `--demo`
- One test root per component under `tests/qml/<Component>/`, run by `make test-qml`
- No compositor or Docker-based end-to-end tests: they were slow and caught little. Manual checks happen in the installed plugin (`make install-local`)
