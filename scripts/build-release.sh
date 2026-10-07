#!/usr/bin/env sh
# Build the release helpers for every supported architecture into
# build/release/, with SHA256SUMS, the GPL-3.0 text and the third-party
# license notices (see docs/decisions.md for why a release binary needs
# both: it links go.mau.fi/libsignal, which is GPL-3.0). The release
# workflow publishes these files; they are never committed. Run
# `make third-party-notices` first to regenerate build/THIRD_PARTY_NOTICES;
# `make build-all` does this for you.
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
out=build/release
notices=build/THIRD_PARTY_NOTICES
[ -f "$notices" ] || { printf '%s is missing; run "make third-party-notices" first\n' "$notices" >&2; exit 1; }
rm -rf "$out"
mkdir -p "$out"

for arch in amd64 arm64; do
    printf 'Building Linux %s helper...\n' "$arch"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
        go build -trimpath -buildvcs=false -ldflags='-s -w' \
        -o "$out/oma-messenger-service-linux-$arch" ./backend
done
cp LICENSE-GPL-3.0 "$out/LICENSE-GPL-3.0"
cp "$notices" "$out/THIRD_PARTY_NOTICES"
(cd "$out" && sha256sum oma-messenger-service-linux-* LICENSE-GPL-3.0 THIRD_PARTY_NOTICES > SHA256SUMS)
printf 'Built %s helpers in %s\n' "$(tr -d ' \n' < helper-version)" "$out"
