# OmaMessenger implementation spec

Status: Telegram and WhatsApp connectors and the plugin UI are built; the 0.3.0 release is being prepared. Build plan: `docs/plan.md`.

## Objective

Build an Omarchy-native, keyboard-first messaging client with a decoupled Go service. Support WhatsApp and Telegram through proven Go clients, with normalized accounts, conversations, and messages so other services can be added later.

## Product boundaries

- The QML plugin entry point is loaded by the existing `omarchy-shell` process and opens a standard Quickshell `FloatingWindow`, managed by Hyprland as a normal non-modal application window.
- A small Go helper process owns protocol connections and local persistence. Omarchy's shell starts and supervises it through the plugin service entry point; the UI and helper communicate over JSON-RPC 2.0 on the helper's stdin and stdout.
- Linux x86_64 and ARM64 helper binaries are published as GitHub Release assets with `SHA256SUMS`, never committed. `helper-version` pins the exact release. The window installs it the first time it opens and finds the helper missing or pinned to a different version, downloading and verifying it into the user's data directory with no click needed; `scripts/install-helper.sh` runs the same installer by hand, and a Retry button is the manual path after a failed automatic attempt. Loading the plugin never downloads anything, and no compiler or system service is needed. `Service.qml` starts the helper through the `bin/oma-messenger-service` launcher.
- Protocol sessions and the SQLite database stay under the user's data directory.
- The MVP starts with text conversation workflows. Media and reactions come after stable text send/receive.
- Do not implement WhatsApp or Telegram protocols from scratch and do not fork Omarchy.

## MVP acceptance

- WhatsApp and Telegram accounts can be authenticated and kept across restarts.
- One unified conversation list shows service, preview, and unread count.
- The normal application window separates service navigation, the conversation list, and the active conversation into clear panes.
- A conversation view loads persisted messages and can send text through the owning service connector.
- Incoming text is persisted and shown without restarting the UI; unread state and desktop notifications update.
- Search filters conversations and messages.
- Multiple accounts are distinguishable and can be switched from keyboard and mouse.
- `j` / `k`, `Enter`, `Esc`, `Ctrl+K`, `Ctrl+N`, and service/account switching work.
- The panel follows active Omarchy `Color` and `Style` theme tokens.
- The window can receive focus on summon, yield focus when the user switches away, move across workspaces, and close through Escape, its Close button, or the compositor window-close action.
- README explains installation, sign-in and the essential shortcuts, linking to `docs/shortcuts.md`, `docs/telegram.md` and `CONTRIBUTING.md` for the rest.

## Failure and privacy behavior

- Show disconnected, authenticating, and connector error states in the UI.
- Keep the local IPC private to the user session and validate request payloads.
- Never expose authentication capabilities through the third-party Quickshell plugin API.
- Do not include message content or secrets in logs.
- Keep adapters behind a generic connector interface and avoid service-specific shapes in QML.

## Known limitations

- WhatsApp's older history does not load further back as you scroll; only what syncs when the device is linked is available.
- The helper does not yet catch up on everything that happened while it was offline; a long gap between runs may miss updates until the next full sync.
- Files attached to outgoing messages are kept in `media/outgoing/` for retries and are not yet cleaned up after delivery.
