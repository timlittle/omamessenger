# OmaMessenger implementation spec

Status: MVP scaffold in progress

## Objective

Build an Omarchy-native, keyboard-first messaging client with a decoupled Go service. Support WhatsApp and Telegram through proven Go clients, with normalized accounts, conversations, and messages so other services can be added later.

## Product boundaries

- The QML plugin entry point is loaded by the existing `omarchy-shell` process and opens a standard Quickshell `FloatingWindow`, managed by Hyprland as a normal non-modal application window.
- A small Go helper process owns protocol connections and local persistence. Omarchy's shell starts and supervises it through the plugin service entry point; the UI and helper communicate over local JSON-lines IPC on stdio.
- Linux x86_64 and ARM64 helper binaries ship with the plugin. `Service.qml` selects and starts the bundled helper; plugin installation needs no compiler, download step, or system service.
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
- README explains setup, authentication, installation, shortcuts, and development.

## Failure and privacy behavior

- Show disconnected, authenticating, and connector error states in the UI.
- Keep the local IPC private to the user session and validate request payloads.
- Never expose authentication capabilities through the third-party Quickshell plugin API.
- Do not include message content or secrets in logs.
- Keep adapters behind a generic connector interface and avoid service-specific shapes in QML.

## Current gap

The UI, JSON-lines IPC, persistence, and Omarchy plugin/helper lifecycle scaffold exist. The seeded demo exercises local sending, scripted delivery states, incoming replies, unread counts, and desktop notifications. Real account authentication, synchronization, delivery, and incoming updates are not implemented yet. Do not describe this scaffold as a usable WhatsApp/Telegram replacement until those acceptance items work end-to-end.
