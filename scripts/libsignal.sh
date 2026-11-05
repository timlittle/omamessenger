#!/usr/bin/env sh
# Fetch and build libsignal-ffi, the Rust library the Signal connector's
# cgo bindings link against, at the exact commit signalmeow pins.
# `make libsignal` calls this; it is a prerequisite of every Go target
# that needs cgo to link.
#
# Usage: libsignal.sh <commit> <version> <arch> <out-dir> <cargo-target-dir> <jobs>
#
# <commit> is checked out detached and verified, never trusted from a tag:
# a moved or re-pushed tag must never silently swap in different source.
# The build is skipped when <out-dir>/libsignal_ffi.a already exists, so a
# repeat `make check` does not rebuild it. rustup picks up the toolchain
# libsignal's own rust-toolchain file names; cmake, protoc and clang build
# its native dependencies (see CONTRIBUTING.md).
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
