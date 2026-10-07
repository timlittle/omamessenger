## Privacy and Security

- Account sessions, contacts and messages stay on this machine, under `$XDG_DATA_HOME/omamessenger` (never inside the plugin directory, which Omarchy hot-reloads)
- Never log credentials, QR tokens, session keys, phone numbers or message bodies. Log IDs, counts and states instead
- `tools/nologcontent` enforces this for the Go code; never silence it. Fix the log call instead
- The helper never makes network requests other than to the messaging services
- Nothing downloads on plugin load. Opening the window is what triggers the install, the first time the helper is missing or pinned to a different version; `make install-helper` runs the same verified installer by hand. Its checksum and version are always verified
- Never commit helper binaries, databases, session files or test data taken from a real account
- Database and session files are created with `0600`, their directories with `0700`
- Installation stays user-scoped: no `sudo`, no writes to `/usr`, no changes to Omarchy's packaged source or system config
