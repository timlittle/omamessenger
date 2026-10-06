#!/usr/bin/env sh
# Build the release helpers for every supported architecture into
# build/release/, with SHA256SUMS. The release workflow publishes these files;
# they are never committed.
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
out=build/release
rm -rf "$out"
mkdir -p "$out"

for arch in amd64 arm64; do
    printf 'Building Linux %s helper...\n' "$arch"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
        go build -trimpath -buildvcs=false -ldflags='-s -w' \
        -o "$out/oma-messenger-service-linux-$arch" ./backend
done
(cd "$out" && sha256sum oma-messenger-service-linux-* > SHA256SUMS)
printf 'Built %s helpers in %s\n' "$(tr -d ' \n' < helper-version)" "$out"
