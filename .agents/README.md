# AI work in OmaMessenger

Use the repository root `AGENTS.md` as the working instructions and `FORGE_SPEC.md` as the product contract. Keep both files in sync with meaningful architecture or scope changes.

## Suggested workflow

1. Read both files and inspect the current code before proposing an implementation.
2. Turn the relevant acceptance points into concrete changes within this repository.
3. Keep protocol integrations behind the Go connector boundary; keep credentials and message data local.
4. Use `make test` for the local build, Go coverage/API tests, QML lint, and isolated Docker window checks. Use `make validate` for the Omarchy plugin manifest. Do not use the developer's active Hyprland session for automated testing.
5. Do not describe a scaffold as a working messaging client while remote login, synchronization, and delivery are missing.

Omarchy's `omarchy agent prompt` can run agent work asynchronously. Use it only after a human has reviewed a completed spec and accepted its scope; the current spec is marked in progress and is not ready for an unattended implementation run.
