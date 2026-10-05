# OmaMessenger

OmaMessenger is an Omarchy-native, keyboard-first messaging client. Omarchy summons its normal, non-modal Quickshell window; the UI and Go helper communicate through a loopback HTTP API. The helper owns persistence and the protocol adapter boundary; the client uses normalized accounts, conversations, and messages.

## Current status

The repository contains the Omarchy plugin entry point, keyboard navigation, a local Go API, and SQLite storage. The entry point opens a standard Hyprland-managed window that can be moved, closed, or left on another workspace. The shell starts the bundled Go helper as a child process automatically. The current service stores accounts and messages locally. WhatsApp and Telegram protocol adapters, QR/code authentication, remote conversation sync, and delivery are not connected yet; messages sent through this scaffold are local records. It cannot replace the desktop clients until those adapters work end to end.

The planned Go clients are `whatsmeow` for WhatsApp and `gotd/td` for Telegram. Media and reactions follow a reliable text workflow.

## Requirements

- Omarchy with `omarchy-shell` and the third-party plugin manager
- Linux x86_64 or ARM64

## Install on Omarchy

Install the plugin with Omarchy:

```sh
omarchy plugin add https://github.com/timlittle/omamessenger --enable
```

Open or focus OmaMessenger:

```sh
omarchy-shell shell summon io.github.omamessenger '{}'
```

The bundled helper starts automatically when the plugin loads in `omarchy-shell`; no Go compiler or separate service setup is needed. The binaries currently target Linux x86_64 and ARM64. There is no separate installer or systemd unit.

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

The root Makefile provides the regular development workflow:

```sh
make help             # list all targets
make build            # build the helper for this machine
make test             # build, run backend/keyboard tests, coverage, and lint
make status           # inspect the current worktree
make pull             # fast-forward from the configured upstream
make install-local    # copy this checkout into Omarchy and enable it
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for project boundaries, the test map, UI review guidance, and pull request expectations. `AGENTS.md` contains the working rules used by both human and AI contributors.

`make test` requires Go, Node, and QML lint tools. The Go API and SQLite tests run in process with temporary databases. `make coverage` enforces at least 80% statement coverage for core Go code, excluding only `main()` process startup wiring. `make test` does not launch a compositor or verify rendered pixels; inspect visual changes on Omarchy with `make install-local`.

`make install-local` builds the native helper, stages the plugin runtime files into `~/.config/omarchy/plugins/io.github.omamessenger/`, validates it, asks the running Omarchy shell to rescan, and enables the plugin. Then open it with `omarchy-shell shell summon io.github.omamessenger '{}'`. This installs your current working tree so you can try changes before pushing.

Build both bundled Linux helper binaries from source with Go 1.23 or newer:

```sh
./scripts/build-release.sh
```

GitHub Actions rebuilds and commits the bundled binaries when backend source changes on `main`. Pushing a `v*` tag also creates a GitHub Release with both architecture builds attached. The repository’s Actions workflow permissions must allow read and write access to contents for the binary commit and release steps. Plugin installs use the binaries tracked in the repository, so no release download or first-run setup is needed.

## Architecture

- `backend/`: Go HTTP API, normalized domain, SQLite persistence, and connector boundary
- `Panel.qml`: Omarchy-summoned Quickshell panel
- `Service.qml`: shell-owned Go helper lifecycle and automatic startup
- `bin/`: bundled Linux x86_64 and ARM64 helpers plus architecture selector
- `scripts/build-release.sh`: offline cross-build for bundled helpers
- `Makefile`: build, test, coverage, validation, local install, status, and pull targets
- `manifest.json`: Omarchy plugin manifest
- `vendor/`: pinned Go dependencies for offline builds

The panel and service entry points run inside the existing `omarchy-shell` process. The Go helper is a child process supervised by the shell and communicates with the UI only through the local API. Theme colors and styles come from Omarchy's live tokens.
