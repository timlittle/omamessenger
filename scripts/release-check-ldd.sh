#!/usr/bin/env sh
# Fail if the release helper's dynamic links go beyond what a stock glibc
# system is expected to carry. `make release-check` calls this right
# after a fresh build-all, so a dependency that needs a library the
# release runners did not ship for cannot slip into a release unnoticed
# (see docs/decisions.md for why each release runner builds natively).
#
# Usage: release-check-ldd.sh <binary>
set -eu

binary=$1
allowed='linux-vdso.so.1|libc\.so|libm\.so|libz\.so|libstdc\+\+\.so|libgcc_s\.so|libresolv\.so|libdl\.so|/lib64/ld-linux|/lib/ld-linux'

unexpected=$(ldd "$binary" | awk '{print $1}' | grep -Ev "^($allowed)" || true)
if [ -n "$unexpected" ]; then
    printf 'release-check: %s links unexpected libraries:\n%s\n' "$binary" "$unexpected" >&2
    exit 1
fi
printf 'release-check: %s links only the allowed system libraries\n' "$binary"
