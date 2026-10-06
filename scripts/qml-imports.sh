#!/usr/bin/env sh
# Prepare a Quickshell-style import directory for linting our UI QML.
#
# qmllint resolves `import qs.Commons` / `import qs.Ui` the same way
# Quickshell does: by finding `Commons` and `Ui` directories under a `qs`
# directory on its import path. This script creates
# build/qml/qs/{Commons,Ui} as symlinks to the real Omarchy shell's
# Commons and Ui directories, so `qmllint -I build/qml` can resolve them
# without copying any Omarchy source.
#
# SHELL_DIR defaults to ${OMARCHY_PATH:-/usr/share/omarchy}/shell, and can be
# overridden directly with OMARCHY_SHELL_DIR (e.g. for a non-standard
# Omarchy install or a test fixture).
#
# Safe to re-run: it replaces any existing symlinks at the target paths.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
shell_dir="${OMARCHY_SHELL_DIR:-${OMARCHY_PATH:-/usr/share/omarchy}/shell}"

fail() { printf 'qml-imports.sh: %s\n' "$1" >&2; exit 1; }

[ -d "$shell_dir/Commons" ] || fail "no Commons directory at '$shell_dir/Commons' (set OMARCHY_SHELL_DIR or OMARCHY_PATH)"
[ -d "$shell_dir/Ui" ] || fail "no Ui directory at '$shell_dir/Ui' (set OMARCHY_SHELL_DIR or OMARCHY_PATH)"

qs_dir="$repo_root/build/qml/qs"
mkdir -p "$qs_dir"

ln -sfn "$shell_dir/Commons" "$qs_dir/Commons"
ln -sfn "$shell_dir/Ui" "$qs_dir/Ui"
