# OmaMessenger agent guide

OmaMessenger is an Omarchy third-party Quickshell plugin backed by a Go helper process. Its IPC-summoned entry point opens a normal, non-modal client window. Preserve the seam between the QML UI, local JSON-lines IPC, and each service connector.

## Project rules

- Do not edit or fork files in Omarchy's packaged source. The install target is the user's `~/.config/omarchy/plugins/io.github.omamessenger/` directory.
- Keep the UI inside the existing long-lived `omarchy-shell` process, but use a standard `FloatingWindow` so Hyprland can manage it like an application. Do not create a full-screen layer-shell overlay or start another Quickshell process.
- Let Omarchy own the helper through `Service.qml`. Ship Linux x86_64 and ARM64 helper binaries in `bin/`; the service selects and starts the bundled binary automatically. Do not add systemd units or plugin installation hooks.
- Keep WhatsApp and Telegram protocol code in Go connectors. QML should use only the normalized local API.
- Keep account sessions and message data local. Do not log credentials, QR tokens, message bodies, or session keys.
- Use Omarchy's `qs.Commons` theme tokens and the installed `qs.Ui` components where they fit the interaction.
- Keep the UI keyboard-first and mouse usable. Preserve the documented shortcuts when changing navigation.
- Keep installation user-scoped. Do not use `sudo`, write to `/usr`, or change system Omarchy configuration from project scripts.
- Update the README when setup, authentication, installation, API behavior, or shortcuts change.

## Interface layout

- Use three clear areas: a narrow service rail, the unified conversation list, and the active conversation view.
- Keep service filters in the rail and search beside the conversation list. Do not let navigation controls crowd message content.
- Use Omarchy theme tokens for colors, spacing, type, and popups. Keep controls visually quiet until selected, hovered, or focused.
- The service rail represents providers today. Add account switching there when the account API and UI behavior are ready; do not imply account switching is implemented before then.
- Preserve a usable list and message pane at the minimum window size. Check long conversation names, empty states, unread badges, and the compose flow when adjusting pane widths.

## Layout

- `Panel.qml`, `manifest.json`: Omarchy plugin entry point and panel UI.
- `backend/`: Go JSON-lines helper, normalized domain, persistence, and service connectors.
- `tests/`: pure logic and isolated compositor checks.
- `scripts/`: build, test, coverage, and local installation helpers.
- `Service.qml`: shell-owned lifecycle for the Go helper.
- `vendor/`: vendored Go dependencies used to produce release binaries offline.
- `bin/`: bundled Linux helper binaries and architecture-selecting launcher.
- `scripts/build-release.sh`: reproducible cross-build for supported Linux architectures.

## Development

The project has no repository-specific AI framework dependencies. Read `FORGE_SPEC.md` for the product contract and acceptance boundaries. On Omarchy, use `omarchy plugin validate .`, `qmllint -I "$OMARCHY_PATH/shell" Panel.qml`, and the Go build command when the user requests validation or the implementation task calls for it.
