# OmaMessenger

OmaMessenger is an Omarchy-native, keyboard-first messaging panel. Its Quickshell UI and Go helper communicate through a loopback HTTP API. The helper owns persistence and the protocol adapter boundary; the panel uses normalized accounts, conversations, and messages.

## Current status

The repository contains the Omarchy panel, keyboard navigation, a local Go API, and SQLite storage. The shell starts the Go helper as a child process after the user builds it once from the panel. The current service stores accounts and messages locally. WhatsApp and Telegram protocol adapters, QR/code authentication, remote conversation sync, and delivery are not connected yet; messages sent through this scaffold are local records. It cannot replace the desktop clients until those adapters work end to end.

The planned Go clients are `whatsmeow` for WhatsApp and `gotd/td` for Telegram. Media and reactions follow a reliable text workflow.

## Requirements

- Omarchy with `omarchy-shell` and the third-party plugin manager
- Go 1.23 or newer to build the helper (dependencies are vendored)

## Install on Omarchy

After this repository has a public Git remote, install the plugin with Omarchy:

```sh
omarchy plugin add https://github.com/OWNER/omamessenger --enable
```

Open OmaMessenger:

```sh
omarchy-shell shell summon io.github.omamessenger '{}'
```

On first launch, choose **Build helper**. This compiles the vendored Go source in the installed plugin directory and starts it under `omarchy-shell`. Future shell starts launch the built helper automatically. If Go is missing, install it with `omarchy pkg add go`, then choose **Build helper** again. There is no separate installer or systemd unit.

This workspace does not have a configured Git remote, so replace `OWNER` with the actual repository owner after publishing it.

Remove the plugin with:

```sh
omarchy plugin remove io.github.omamessenger
```

## Accounts and authentication

The API accepts WhatsApp and Telegram account records, but remote authentication is not implemented. For development, read the API bearer token and add a local account record:

```sh
read -r OMA_TOKEN < ~/.config/omamessenger/api.token
curl -X POST http://127.0.0.1:43821/api/v1/accounts \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $OMA_TOKEN" \
  -d '{"id":"personal-wa","service":"whatsapp","name":"Personal WhatsApp"}'
```

This creates a local record only; it does not establish a WhatsApp or Telegram session.

SQLite lives at `~/.config/omamessenger/messages.db` (`OMA_DB` changes the path). The API listens on `127.0.0.1:43821` (`OMA_PORT` changes the port; update `Panel.qml` to match). A random bearer token is created at `~/.config/omamessenger/api.token` with owner-only permissions and read by the panel.

## Keyboard shortcuts

- `j` / `k`: move through conversations
- `Enter`: open the selected conversation
- `Esc`: return to the list, then close the panel
- `Ctrl+K`: focus conversation search
- `Ctrl+N`: create a local conversation
- `Ctrl+1` / `Ctrl+2` / `Ctrl+0`: WhatsApp / Telegram / all conversations
- Mouse clicks work for filters, conversation rows, and message controls

The compose dialog expects an account ID and contact/chat label. Without service connectors, it creates only a local conversation.

## Development

Run the API directly:

```sh
go run -buildvcs=false ./backend
```

The API is rooted at `http://127.0.0.1:43821/api/v1`; authenticated requests use `Authorization: Bearer <token>`. `POST /events/message` is the normalized incoming-message boundary for future connectors and triggers a desktop notification.

Validate the plugin and QML on Omarchy:

```sh
omarchy plugin validate .
qmllint -I "$OMARCHY_PATH/shell" Panel.qml Service.qml
```

Then rescan and open it:

```sh
omarchy-shell shell rescanPlugins
omarchy-shell shell summon io.github.omamessenger '{}'
```

## Architecture

- `backend/`: Go HTTP API, normalized domain, SQLite persistence, and connector boundary
- `Panel.qml`: Omarchy-summoned Quickshell panel
- `Service.qml`: shell-owned Go helper lifecycle and explicit first build
- `manifest.json`: Omarchy plugin manifest
- `vendor/`: pinned Go dependencies for offline builds

The panel and service entry points run inside the existing `omarchy-shell` process. The Go helper is a child process supervised by the shell and communicates with the UI only through the local API. Theme colors and styles come from Omarchy's live tokens.
