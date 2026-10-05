# Contributing to OmaMessenger

OmaMessenger is an Omarchy plugin first and a messaging client second. Keep it native to the running Omarchy shell, let Hyprland manage its application window, and keep messaging protocols behind the Go service boundary.

## Before changing code

- Read [AGENTS.md](AGENTS.md) for the project working agreements and [FORGE_SPEC.md](FORGE_SPEC.md) for product scope and acceptance criteria.
- Keep the Quickshell plugin UI, local JSON-lines IPC, persistence, and service connectors independently understandable. The UI consumes normalized accounts, conversations, and messages; it must not speak WhatsApp or Telegram protocol details.
- A service-specific capability belongs in the service connector. Do not show a control that suggests an operation is available when that service cannot perform it.
- Keep credentials and message content out of logs, shell configuration, and test output. Use temporary directories and fake service data in tests.

## UI and interaction

The conversation window uses three areas: a service navigation rail, the unified conversation list, and the active conversation. Keep these panes useful at the supported window sizes, and make the same actions available by keyboard and mouse.

Use Omarchy theme tokens and existing Quickshell UI components. Preserve the documented shortcuts, keep focus inside the active window, and make Escape and the window close control behave predictably. When changing a visible interaction, update the shortcut documentation and include the affected states in verification.

## Tests

Add a regression test with each behavior fix. Put the test at the narrowest layer that can observe the behavior:

| Change | Test location |
| --- | --- |
| Pure keyboard or data transformation logic | `tests/unit/`, runnable with Node |
| Go persistence, RPC framing, or helper behavior | focused `backend/internal/*/*_test.go` or `backend/*_test.go`, using temporary SQLite databases and in-process JSON-lines streams |
| Plugin manifest or QML validity | `make validate` and `make lint` |

Run the relevant test once against the failing behavior before fixing it when practical. Before opening a pull request, run:

```sh
make test
```

This includes the 80% core Go statement coverage gate, JSON-lines RPC and persistence tests, keyboard logic tests, and lint. It does not start a compositor or claim to verify rendered QML. Review visual changes on Omarchy with `make install-local`, using both keyboard and mouse.


## Checking visual changes

Automated QML checks cover syntax. They do not replace looking at the rendered interface or testing window behavior. Use `make install-local` on an Omarchy development machine, open the plugin, and check the changed flow with both keyboard and mouse. When practical, include the Omarchy theme and window size used, plus a screenshot, in the pull request. Do not claim a live desktop check was performed if it was not.

## Pull requests

Keep each pull request focused on one user-visible change or one maintenance task. Include:

- The problem and a short reproduction or design reason.
- The behavior changed and any service-specific limitations.
- Exact verification commands and their results, including whether Docker UI checks ran.
- A screenshot for visual changes when available.

If an AI coding tool helped, the contributor remains responsible for reviewing the full diff and understanding each change. Follow the repository's `AGENTS.md`; do not invent a second set of agent-only project rules.
