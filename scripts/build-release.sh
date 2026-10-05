#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
mkdir -p bin

for arch in amd64 arm64; do
    printf 'Building Linux %s helper...\n' "$arch"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
        go build -mod=vendor -trimpath -buildvcs=false -ldflags='-s -w' \
        -o "bin/oma-messenger-service-linux-$arch" ./backend
done
