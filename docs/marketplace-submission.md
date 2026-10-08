# Omarchy Plugin Marketplace submission (draft)

This is a draft of OmaMessenger's submission to the
[omacom/omarchy-plugin-marketplace](https://github.com/omacom/omarchy-plugin-marketplace),
prepared against that repository's `SUBMISSION.md`, `.github/ISSUE_TEMPLATE/submit-plugin.yml`
and `SECURITY.md` (read read-only via the GitHub API; not fetched into this
repository). **Nothing here has been submitted.** It exists so the real
issue can be filed later by copying the "Issue body" section below, after
the owner has reviewed and confirmed the attestations.

## Listing metadata

| Field | Value |
| --- | --- |
| Plugin name | OmaMessenger |
| Plugin ID | `io.github.omamessenger` |
| Repository URL | https://github.com/timlittle/omamessenger |
| Category | Productivity |
| Tags | `quickshell`, `bar`, `media` |
| Short description | A keyboard-first messaging client for Omarchy, for Telegram and WhatsApp accounts. |
| Preview image | `preview.png` at the repository root (already present) |

The ID is outside the reserved `omarchy.*` namespace and has not been checked
against the live marketplace catalog for a collision yet (`registry.json`
has no existing messaging-plugin entry, from a search run for this draft) —
do that check again just before filing.

**Category**: no other messaging client is listed yet to copy from. Of the
marketplace's fixed categories (Appearance, Desktop, Developer Tools,
Hardware, Kids, Productivity, System, Widgets, Other), "Productivity" fits
a messaging client better than "Desktop" or "Widgets" (OmaMessenger has a
bar widget, but the window is the product, not a bar accessory).

**Tags**: chosen from the allowed list in `submit-plugin.yml`. `quickshell`
because the plugin is a Quickshell `FloatingWindow` entry point; `bar`
because it ships a bar-widget entry point with an unread count; `media`
because it sends and receives photos, video, voice notes and files. No tag
in the allowed list names messaging/chat directly, so no "Suggest a missing
tag" entry is included below — reconsider that if a `messaging` or `chat`
tag exists by the time this is filed.

## Install and removal

From the README:

```sh
omarchy plugin add https://github.com/timlittle/omamessenger --enable
```

The first time the window opens, it downloads and verifies the helper
release pinned in `helper-version`, then starts it. Adding the plugin also
registers it in Omarchy's apps menu (**SUPER+ALT+SPACE**).

```sh
omarchy plugin remove io.github.omamessenger
rm -rf ~/.local/share/omamessenger
rm -f ~/.local/share/applications/io.github.omamessenger.desktop
```

Plus removing any `o.bind(...)` line the user added to
`~/.config/hypr/bindings.lua` for a custom shortcut. Both sequences are
already documented in the README's Install and Uninstall sections.

## Background processes and network access

- **Background process**: a Go helper (`oma-messenger-service`), started by
  `ui/Service.qml` through Quickshell's `Process` the first time the window
  opens, and stopped when the window's service is destroyed (plugin
  disable/removal, or a plugin-folder reload). It talks to the UI only over
  its own stdin/stdout (JSON-RPC), never over a socket.
- **Network access**:
  - Telegram's own servers (MTProto, via `gotd/td`) and WhatsApp's own
    servers (via `go.mau.fi/whatsmeow`) — only for accounts the user has
    signed in.
  - `github.com/timlittle/omamessenger/releases` — only to download the
    helper binary, only the first time the window opens and finds the
    helper missing or pinned to a different version than `helper-version`;
    the download is HTTPS-only, size-capped, and checksum- and
    version-verified before the binary is ever run or moved into place
    (`scripts/install-helper.sh`). Loading or enabling the plugin triggers
    no download by itself.
  - No other network access. No telemetry, no analytics, no third-party
    service beyond the two messaging services and that one release host.
- **No `sudo`, no `pkexec`**, anywhere in the repository (checked by grep
  across `scripts/` and `ui/` for this draft) — every install and data
  operation is user-scoped.
- **No systemd units, no package-manager invocations** in any script.

## Data locations and permissions

| What | Where | Mode |
| --- | --- | --- |
| Accounts, chats and messages | `$XDG_DATA_HOME/omamessenger/messages.db` | file `0600`, directory `0700` |
| Telegram sessions and API keys | `$XDG_DATA_HOME/omamessenger/telegram/` | files `0600`, directory `0700` |
| WhatsApp sessions and media references | `$XDG_DATA_HOME/omamessenger/whatsapp/` | files `0600`, directory `0700` |
| Cached and outgoing photos, video and files | `$XDG_DATA_HOME/omamessenger/media/` | files `0600`, directory `0700` |
| Installed helper binary | `$XDG_DATA_HOME/omamessenger/bin/` | executable, directory `0700` |
| Plugin source (the checkout itself) | `~/.config/omarchy/plugins/io.github.omamessenger/` | as installed, read-only at runtime — no session or message data is ever written here |
| Apps menu entry | `~/.local/share/applications/io.github.omamessenger.desktop` | `0644`, no secrets |
| Key remap file (only if the user asks for it) | `$XDG_CONFIG_HOME/omamessenger/keys.conf` | created only from the command palette's "Open key bindings file"; never touches `~/.config/omarchy` |

Nothing is written outside these locations. The plugin never edits
Omarchy's own configuration or packaged source, and never asks for
elevated privileges. This table matches the README's "What OmaMessenger
reads, writes and sends" section.

## Licence

- OmaMessenger's own source is **MIT** (`LICENSE`).
- Released helper **binaries** additionally link `go.mau.fi/libsignal`
  (GPL-3.0, used for WhatsApp's encryption), so the compiled release
  binaries are distributed under **GPL-3.0** (`LICENSE-GPL-3.0`). This is a
  binary-distribution licence, not a source-licence change: the repository
  stays MIT throughout.
- `THIRD_PARTY_NOTICES` is generated per release (from
  `scripts/third-party-notices.tmpl`) and published next to the binary in
  each GitHub release, listing every third-party dependency the helper
  links.
- The README's License section already states this split; point reviewers
  there.

## Required attestations

From `submit-plugin.yml`'s submission checklist, with how OmaMessenger
meets each one:

- [x] **The repository is public and contains installation and removal
      instructions.** — README's Install and Uninstall sections (quoted
      above).
- [x] **I have documented the plugin license and any external
      dependencies.** — README License section, `LICENSE`,
      `LICENSE-GPL-3.0`, and `THIRD_PARTY_NOTICES` published with each
      release (see Licence above).
- [ ] **I confirm that I own or have permission to submit this plugin and
      its preview assets.** — needs an explicit yes from the repository
      owner (Tim Little) before filing; `preview.png` is this project's own
      screenshot, not a third-party asset.
- [x] **The plugin does not overwrite user configuration without explicit
      consent.** — OmaMessenger never writes to `~/.config/omarchy` or any
      other Omarchy configuration. The only files it creates outside its
      own data directory are the apps-menu `.desktop` entry (standard,
      user-scoped, created on install, removed on uninstall) and
      `keys.conf` (created only when the user explicitly chooses "Open key
      bindings file"). Nothing is overwritten without the user asking for
      it.
- [x] **I understand that approval is for listing and is not a security
      review.** — acknowledged; see also the baseline notes below.

The second item is left unchecked here on purpose: it is the one attestation
only the repository owner can make, and it should be confirmed immediately
before the real issue is filed, not assumed in a draft.

## What the Automated Security Baseline would likely flag

Per `SECURITY.md`'s documented patterns and capabilities, checked against
this repository as it stands on `main`:

- **`installer` capability (expected, not blocking)** — `scripts/install-helper.sh`
  and the `make install-helper` target are named and shaped like an
  installer, so the scanner should detect the `installer` capability and
  mark the result `review-required`. This is expected and benign: the
  script only downloads a release asset over HTTPS, verifies it against
  the release's own `SHA256SUMS`, confirms the binary reports the exact
  pinned version, and only then `mv`s it into place — it never pipes
  downloaded content into a shell, and it needs no `sudo`.
- **`curl-pipe-shell` — not expected.** The repository's one `curl` call
  (`scripts/install-helper.sh`) writes to a file with `--output` and is
  checksum-verified before anything touches it; nothing downloaded is ever
  piped into a shell or an interpreter.
- **`cargo-git-unpinned` / `remote-git-execution-unpinned` — not expected.**
  No script builds or executes code cloned from an external Git repository;
  the helper is a prebuilt release binary downloaded by URL and checksum,
  not built from source at install time.
- **`sudoers-dangerous-passwordless-command` /
  `privileged-process-control-from-shared-temp` — not expected.** No
  `sudo`, `pkexec`, `NOPASSWD`, or shared-`/tmp` PID handoff appears
  anywhere in the repository (checked by grep across `scripts/` and `ui/`
  for this draft).
- **`bundled-executable-binary` — not expected.** No binaries are ever
  committed to the repository; this is an explicit project rule
  (`.claude/rules/scripts.md`, `.claude/rules/privacy.md`) as well as a
  `.gitignore` entry, and the helper is always downloaded, never checked
  in.
- **`service-management` — not expected.** No systemd unit files and no
  `systemctl`/`systemd-run` references anywhere.
- **`privilege` / `sudoers-modification` — not expected.** Same grep as
  above: no `sudo` or `pkexec` reference exists to find.
- **`package-manager` — not expected.** No script invokes `pacman`,
  `apt`, `yay`, `paru`, `dnf` or similar; the optional `qt6-multimedia`
  dependency the README mentions for in-window voice playback is never
  installed by OmaMessenger itself, only checked for at runtime.

Expected outcome: `review-required`, for the `installer` capability alone,
with no documented findings — eligible for `approved-and-verified` once a
maintainer reviews that one capability and accepts it. If a future change
adds a second release asset or a different install path, re-run this
checklist before resubmitting, since the baseline's exact-commit binding
means any new commit needs its own fresh scan.

## Issue body (ready to file, once approved)

This mirrors `SUBMISSION.md`'s required format exactly — same six
headings, same order, same checklist text — for a direct
`gh issue create --repo omacom/omarchy-plugin-marketplace --title "[Plugin]: OmaMessenger" --body-file <this>`
once the owner has confirmed the ownership attestation above.

```markdown
### Repository URL

https://github.com/timlittle/omamessenger

### Category

Productivity

### Tags

quickshell, bar, media

### Suggest a missing tag

_No response_

### Maintainer notes

OmaMessenger is a keyboard-first Telegram and WhatsApp client for Omarchy.
It runs a small Go helper process, started by the plugin's own Service.qml,
that talks only to Telegram's and WhatsApp's servers; the only other
network access is a one-time, checksum-verified download of the helper
binary from this repository's GitHub Releases, the first time the window
opens and finds it missing or out of date (scripts/install-helper.sh).
No sudo, no systemd units, no package-manager calls, and no binaries are
ever committed to the repository. All account sessions and messages are
stored user-scoped under $XDG_DATA_HOME/omamessenger with 0600/0700
permissions; see the README's "Data and privacy" section for the full
breakdown. Expect the Automated Security Baseline to flag the installer
capability on scripts/install-helper.sh (review-required, not a finding) —
see docs/marketplace-submission.md in the repository for why that script
is safe.

### Submission checklist

- [x] The repository is public and contains installation and removal instructions.
- [x] I have documented the plugin license and any external dependencies.
- [x] I confirm that I own or have permission to submit this plugin and its preview assets.
- [x] The plugin does not overwrite user configuration without explicit consent.
- [x] I understand that approval is for listing and is not a security review.
```

The checklist above shows all five boxes checked only because that is the
fixed required text for filing; actually checking them is a decision for
the repository owner, not this draft — see "Required attestations" above,
where the ownership item is deliberately left open.
