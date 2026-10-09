## Machine Resources

Development runs on the developer's own laptop, next to their desktop session. Work that leaks memory, disk or processes has twice degraded it badly (a full `/run/user` tmpfs crashed Hyprland; caches in `/tmp` plus a CI run exhausted RAM). Clean up after every run, success or failure.

- `/tmp`, `/run/user/<uid>` and `/dev/shm` are RAM-backed tmpfs. Never put caches, module downloads, build output, clones or large scratch there. Use `build/` in the checkout (gitignored), or Go's and Docker's own default caches. The Makefile sets `GOTMPDIR` to `build/gotmp` for this reason
- A temporary directory you do need comes from `mktemp -d` and is removed by a `trap` (or `t.TempDir()` in Go tests), so an interrupted run removes it too
- Anything that starts quickshell gives it a throwaway `XDG_RUNTIME_DIR` and deletes it afterwards (see ui-testing.md)
- Docker: always `--rm`, run as the developer (`-u "$(id -u):$(id -g)"`) so nothing in the checkout ends up owned by root, cap memory (`--memory`), and remove every container you started before reporting. Name ad-hoc containers with an `oma-` prefix
- Run heavy jobs one at a time, not side by side: `make ci` runs its jobs in sequence, each capped at `CI_MEMORY`
- `scripts/check-leaks.sh` runs at the end of `make check` and `make ci`, and fails on a leftover `act` container, `oma-*` scratch in `/tmp`, or a root-owned file in the checkout. Fix the leak; never silence the check
- Before pushing, `make ci` must pass (see workflow.md)
