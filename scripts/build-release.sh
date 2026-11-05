#!/usr/bin/env sh
# Build the release helper for this host's own architecture into
# build/release/, with SHA256SUMS, the AGPL-3.0 text and the third-party
# license notices (see docs/decisions.md for why a release binary needs
# both: it links libsignal, which is AGPL-3.0). cgo and libsignal_ffi.a
# cannot cross-compile the way the old CGO_ENABLED=0 build could, so each
# architecture's release now builds natively on its own runner (see
# .github/workflows/release.yml). The release workflow publishes these
# files; they are never committed. Run `make third-party-notices` first to
# regenerate build/THIRD_PARTY_NOTICES and `make libsignal` to build the
# native library this links; `make build-all` does both for you.
set -eu

project_dir=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
out=build/release
notices=build/THIRD_PARTY_NOTICES
[ -f "$notices" ] || { printf '%s is missing; run "make third-party-notices" first\n' "$notices" >&2; exit 1; }
rm -rf "$out"
mkdir -p "$out"

arch=$(go env GOARCH)
printf 'Building Linux %s helper...\n' "$arch"
go build -trimpath -buildvcs=false -ldflags='-s -w' \
    -o "$out/oma-messenger-service-linux-$arch" ./backend
cp LICENSE-AGPL-3.0 "$out/LICENSE-AGPL-3.0"
cp "$notices" "$out/THIRD_PARTY_NOTICES"
(cd "$out" && sha256sum "oma-messenger-service-linux-$arch" LICENSE-AGPL-3.0 THIRD_PARTY_NOTICES > SHA256SUMS)
printf 'Built %s helper in %s\n' "$(cut -d'#' -f1 helper-version | tr -d ' \n')" "$out"
