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

Today:

- `manifest.json`, `Panel.qml`, `Service.qml`, `keyboard.js`: the original root-level plugin scaffold, which Phase B replaces.
- `backend/`: Go JSON-lines helper: `backend/internal/domain`, `store`, `connector` (plus `connector/demo`), `app` (`Commands`, `Ingest`, `app/policy`), `api`, `rpc`, `notify`, and `archtest`.
- `tools/`: rules enforced in code: `tools/omalint` (Go analyzers), `tools/covergate` (coverage gates), `tools/docscheck` (docs checks).
- `tests/unit/`, `tests/e2e/`: launcher and keyboard logic tests, and isolated compositor checks.
- `bin/`: bundled Linux helper binaries and the architecture-selecting launcher, which prefers a local `bin/dev/` build.
- `scripts/build-release.sh`: reproducible cross-build for supported Linux architectures.
- `vendor/`: vendored Go dependencies used to produce release binaries offline.
- `docs/`: the build plan (`docs/TASKS.md`), the generated protocol reference (`docs/PROTOCOL.md`) and the generated architecture metrics (`docs/ARCHITECTURE.md`).

Planned in Phase B (target layout in `docs/TASKS.md` C1):

- `ui/Service.qml`, `ui/Panel.qml`, `ui/BarWidget.qml`: shell lifecycle, window and bar widget (planned).
- `ui/service/`, `ui/controllers/`, `ui/components/`, `ui/lib/`: helper/RPC state, UI actions, views and pure logic (planned).
- `tests/qml/`: offscreen QML integration harness (planned).

## Development

The project has no repository-specific AI framework dependencies. Read `FORGE_SPEC.md` for the product contract and acceptance boundaries, and `docs/TASKS.md` for the build plan, its Definition of Done and its contracts. Architecture rules are enforced in code (`tools/omalint`, `backend/internal/archtest`), and `make docs-check` keeps the docs true to the code. On Omarchy, use `omarchy plugin validate .` and `make lint` when validation is requested. Phase B replaces the transitional QML lint command with `/usr/lib/qt6/bin/qmllint` and a Quickshell import directory prepared by `scripts/qml-imports.sh` (planned).
