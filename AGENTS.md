# OmaMessenger agent guide

OmaMessenger is an Omarchy plugin: a QML UI inside `omarchy-shell`, backed by a Go helper it starts and talks to with JSON-RPC over stdio. Keep the seams clean between the UI, the helper API and each service connector.

## Read first

- [.claude/rules/](.claude/rules/): the coding standards. They apply to every change.
- [docs/plan.md](docs/plan.md): what to build next, as self-contained briefs, with the UI spec.
- [docs/decisions.md](docs/decisions.md): why things are the way they are.
- [FORGE_SPEC.md](FORGE_SPEC.md): product scope and acceptance.

## Project rules

- Do not edit or fork Omarchy's packaged source. The plugin installs into `~/.config/omarchy/plugins/io.github.omamessenger/`.
- Keep the UI inside the running `omarchy-shell`, in a standard `FloatingWindow` that Hyprland manages like an app. No full-screen layer-shell overlay, no second Quickshell process.
- Omarchy owns the helper through the plugin's Service. Never commit helper binaries, and never download anything when the plugin loads; see the README for how the helper is released and installed. No systemd units or plugin installation hooks.
- WhatsApp and Telegram protocol code lives in Go connectors. The UI uses only the helper API.
- Account sessions and messages stay on this machine. Never log credentials, QR tokens, message text or session keys.
- Keep installation user-scoped: no `sudo`, no writes to `/usr`, no changes to system Omarchy configuration.
- Never test against the developer's running Hyprland session.
- Update the README when setup, authentication or installation changes, `docs/shortcuts.md` when a shortcut changes, and `docs/api.md` when the helper API changes.

## Interface

- Three areas: a narrow service rail, the unified conversation list and the open conversation. Service filters live in the rail, search beside the list.
- Keyboard-first and mouse usable. Keep the documented shortcuts working when changing navigation.
- Omarchy theme tokens for colour, spacing, type and popups. Controls stay quiet until hovered, focused or selected.
- The rail shows services today. Do not imply account switching until the account API exists.
- Keep the list and conversation usable at the minimum window size: check long names, empty states, unread badges and composing.

## Layout

- `backend/`: the Go helper. `main.go` wires everything; `internal/domain` (shared types), `store` (SQLite), `connector` (the service boundary and its supervisor, with `fake` for tests), `app` (what the client does: `Commands` for the UI, `Ingest` for connectors, `policy` for notifications), `server` (JSON-RPC), `notify` (desktop notifications).
- `ui/`: the UI: `theme/`, `components/`, `lib/`, `Service.qml`, `Panel.qml`, `BarWidget.qml`. Planned: `service/`, `controllers/` (see the plan).
- `manifest.json` in the root: the plugin manifest, pointing at the entry points in `ui/`.
- `bin/oma-messenger-service`: the launcher. `scripts/`: helper install, release build, local install, QML lint imports.
- `tools/nologcontent`: keeps message content out of logs.
- `tests/unit/`: node tests for `ui/lib` and the scripts.

## Working

- `make check` runs every gate: build, Go tests with the race detector and coverage gates, JavaScript tests, golangci-lint, the privacy check, shellcheck and Qt 6 qmllint. It must pass before work is done.
- `make install-local` installs the checkout into Omarchy for trying the UI.
- `make validate` checks the plugin files as they would be installed.
