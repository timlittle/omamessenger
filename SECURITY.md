# Security

## Reporting a vulnerability

Report a vulnerability through GitHub's private vulnerability reporting: open the [Security tab](https://github.com/timlittle/omamessenger/security) on this repository and click **Report a vulnerability**. This opens a private advisory that only maintainers can see; do not open a public issue for a security problem.

Include what you found, how to reproduce it, and the version of the helper and plugin you tested. Do not include message contents, phone numbers, QR codes or session files from a real account in the report.

## Scope

In scope:

- The Go helper (`backend/`), including the Telegram and WhatsApp connectors and their handling of sessions, credentials and message data
- The Omarchy plugin (`ui/`, `manifest.json`) and the install and release scripts (`scripts/`)

Out of scope:

- Telegram's own servers and infrastructure
- Vulnerabilities that require physical access to an already-unlocked machine

## Rewards

There is no bounty program. Reports are reviewed and fixed on a best-effort basis.

## Verifying a release binary

Each release's helper binaries are built and signed by the `release.yml` workflow on GitHub-hosted runners, never on a developer's machine, and their build provenance is attested with `actions/attest-build-provenance`. Verify a downloaded binary matches that attested build:

```sh
gh attestation verify oma-messenger-service-linux-amd64 -R timlittle/omamessenger
```

This confirms the file was produced by this repository's release workflow from the commit tagged for that release, not altered afterwards. `scripts/install-helper.sh` checks the release's `SHA256SUMS` on every install; attestation verification is an extra, optional check for anyone who wants to confirm the binary's build origin as well as its checksum.
