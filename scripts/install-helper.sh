#!/usr/bin/env sh
# Install the OmaMessenger helper release pinned in helper-version.
#
# The window runs this itself the first time it opens and finds the helper
# missing or pinned to a different version (and again if that Retry button
# is clicked after a failed attempt); `make install-helper` runs the same
# script by hand. Loading the plugin never downloads anything. The binary
# comes from the immutable GitHub release tag, must match the release's
# SHA256SUMS and report the pinned version, and is moved into place
# atomically. It lives outside the plugin directory, so installing it
# neither hot-reloads the plugin nor dirties its git checkout.
#
#   install-helper.sh            install (no-op if already installed)
#   install-helper.sh --status   exit 0 if installed, 1 if not
#
# OMA_RELEASE_BASE overrides the download location (tests use file://).
set -eu

plugin_dir=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=SCRIPTDIR/desktop-entry.sh
. "$plugin_dir/scripts/desktop-entry.sh"
# helper-version carries a trailing "# x-release-please-version" comment
# that release-please's generic updater matches on; cut it before the
# comment, then trim the rest.
version=$(cut -d'#' -f1 "$plugin_dir/helper-version" | tr -d ' \n')
case $(uname -m) in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) printf 'OmaMessenger has no helper for this architecture: %s\n' "$(uname -m)" >&2; exit 1 ;;
esac
data_dir="${XDG_DATA_HOME:-$HOME/.local/share}/omamessenger"
bin_dir="$data_dir/bin"
dest="$bin_dir/oma-messenger-service-$version"
name="oma-messenger-service-linux-$arch"
base="${OMA_RELEASE_BASE:-https://github.com/timlittle/omamessenger/releases/download/v$version}"

fail() { printf 'Helper install failed: %s\n' "$1" >&2; exit 1; }

installed() { [ -x "$dest" ] && [ "$("$dest" --version 2>/dev/null)" = "$version" ]; }

if [ "${1-}" = "--status" ]; then
    if installed; then printf 'installed %s\n' "$version"; exit 0; fi
    printf 'not installed (pinned %s)\n' "$version"; exit 1
fi
if installed; then
    printf 'OmaMessenger helper %s is already installed.\n' "$version"
    install_desktop_entry
    exit 0
fi

# Only https, except a file:// base for tests.
protocols='=https'
case "$base" in file://*) protocols='=file' ;; esac
fetch() { curl --fail --silent --show-error --location --proto "$protocols" --proto-redir '=https' \
    --connect-timeout 10 --max-time 300 --max-filesize "$2" --output "$3" "$1"; }

mkdir -p "$bin_dir"
chmod 700 "$data_dir" "$bin_dir"
work=$(TMPDIR="$bin_dir" mktemp -d)
trap 'rm -rf "$work"' EXIT

printf 'Downloading OmaMessenger helper %s for %s...\n' "$version" "$arch"
fetch "$base/$name" 67108864 "$work/$name" || fail "could not download $name"
fetch "$base/SHA256SUMS" 65536 "$work/SHA256SUMS" || fail "could not download SHA256SUMS"
expected=$(awk -v n="$name" '$2 == n || $2 == "*" n { print $1 }' "$work/SHA256SUMS")
[ -n "$expected" ] || fail "SHA256SUMS has no entry for $name"
actual=$(sha256sum "$work/$name" | cut -d' ' -f1)
[ "$actual" = "$expected" ] || fail "checksum mismatch for $name"
chmod 755 "$work/$name"
reported=$("$work/$name" --version 2>/dev/null) || fail "downloaded helper does not run"
[ "$reported" = "$version" ] || fail "downloaded helper reports version $reported, expected $version"

mv -f "$work/$name" "$dest"
for old in "$bin_dir"/oma-messenger-service-*; do
    [ "$old" = "$dest" ] || rm -f "$old"
done
install_desktop_entry
printf 'Installed OmaMessenger helper %s.\n' "$version"
