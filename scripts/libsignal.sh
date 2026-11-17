#!/usr/bin/env sh
# Fetch and build libsignal-ffi at the pinned commit. `make libsignal`
# calls this.
#
# Usage: libsignal.sh <commit> <version> <arch> <out-dir> <cargo-target-dir> <jobs>
#
# The commit is checked out detached and verified, never trusted from a
# tag. Skips the build when <out-dir>/libsignal_ffi.a already exists.
set -eu

commit=$1
version=$2
arch=$3
out_dir=$4
cargo_target_dir=$5
jobs=$6

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
src_dir="$repo_root/build/libsignal/$version/src"
archive="$repo_root/$out_dir/libsignal_ffi.a"

if [ -f "$archive" ]; then
    printf 'libsignal: %s already built for %s, skipping\n' "$version" "$arch"
    exit 0
fi

# Only a full 40-character commit is ever fetched: a branch or tag can move.
case $commit in
    *[!0-9a-f]* | '') printf 'libsignal: %s is not a full commit hash\n' "$commit" >&2; exit 1 ;;
esac
if [ "${#commit}" -ne 40 ]; then
    printf 'libsignal: %s is not a full 40-character commit hash\n' "$commit" >&2
    exit 1
fi

if [ ! -d "$src_dir" ]; then
    printf 'libsignal: fetching %s at %s...\n' "$version" "$commit"
    mkdir -p "$src_dir"
    git -C "$src_dir" init -q
    git -C "$src_dir" fetch -q --depth 1 https://github.com/signalapp/libsignal "$commit"
    git -C "$src_dir" checkout -q --detach "$commit"
fi

got=$(git -C "$src_dir" rev-parse HEAD)
if [ "$got" != "$commit" ]; then
    printf 'libsignal: checked out %s, want %s\n' "$got" "$commit" >&2
    exit 1
fi

printf 'libsignal: building libsignal-ffi for %s (CARGO_BUILD_JOBS=%s)...\n' "$arch" "$jobs"
(cd "$src_dir" && CARGO_BUILD_JOBS="$jobs" cargo build -p libsignal-ffi --release \
    --target-dir "$cargo_target_dir" -j "$jobs")

mkdir -p "$repo_root/$out_dir"
cp "$cargo_target_dir/release/libsignal_ffi.a" "$archive"
printf 'libsignal: built %s for %s\n' "$version" "$arch"
