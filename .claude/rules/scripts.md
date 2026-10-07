## Scripts and Build

- Prefer an existing tool or a Makefile target to a script. Before writing one, check whether `go`, `golangci-lint`, `go-test-coverage`, `node --test` or `make` already does the job
- Scripts that remain are POSIX `sh` (or `bash` when it needs arrays), start with `set -eu`, and pass `shellcheck` (run by `make lint`)
- Each script starts with a comment saying what it does and who calls it
- Scripts work from any directory: resolve paths from the script's own location
- No `sudo`, no writes outside the repo, `build/`, `~/.local/share/omamessenger`, `~/.local/share/applications` (just the `io.github.omamessenger.desktop` entry) or `~/.config/omarchy/plugins/io.github.omamessenger`
- Go dependencies come from the module cache, not a committed `vendor/` directory. Pin tool versions in the Makefile; `make tools` installs them into `build/tools/`
- Generated files and build output go in `build/` (gitignored). Never commit binaries
- The Makefile is the single entry point. `make check` runs every gate; each target has a `## description` that `make help` shows
