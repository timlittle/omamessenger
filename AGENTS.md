# OmaMessenger agent guide

OmaMessenger is an Omarchy third-party Quickshell plugin backed by a Go helper process. Preserve the seam between the QML panel, the local API, and each service connector.

## Project rules

- Do not edit or fork files in Omarchy's packaged source. The install target is the user's `~/.config/omarchy/plugins/io.github.omamessenger/` directory.
- Keep the panel inside the existing long-lived `omarchy-shell` process. Do not start another Quickshell process.
- Let Omarchy own the helper through `Service.qml`. Build the vendored helper only after an explicit user action; do not add systemd units or plugin installation hooks.
- Keep WhatsApp and Telegram protocol code in Go connectors. QML should use only the normalized local API.
- Keep account sessions and message data local. Do not log credentials, QR tokens, message bodies, or session keys.
- Use Omarchy's `qs.Commons` theme tokens and the installed `qs.Ui` components where they fit the interaction.
- Keep the UI keyboard-first and mouse usable. Preserve the documented shortcuts when changing navigation.
- Keep installation user-scoped. Do not use `sudo`, write to `/usr`, or change system Omarchy configuration from project scripts.
- Update the README when setup, authentication, installation, API behavior, or shortcuts change.

## Layout

- `Panel.qml`, `manifest.json`: Omarchy plugin entry point and panel UI.
- `backend/`: Go HTTP API, normalized domain, persistence, and service connectors.
- `Service.qml`: shell-owned lifecycle for the Go helper.
- `vendor/`: vendored Go dependencies used for offline helper builds.

## Development

The project has no repository-specific AI framework dependencies. Read `FORGE_SPEC.md` for the product contract and acceptance boundaries. On Omarchy, use `omarchy plugin validate .`, `qmllint -I "$OMARCHY_PATH/shell" Panel.qml`, and the Go build command when the user requests validation or the implementation task calls for it.
