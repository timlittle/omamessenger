#!/bin/sh
# Fails when development work has left resources behind on this machine:
# containers from `make ci`, scratch files in the RAM-backed /tmp, or
# root-owned files in the checkout from a container run as root. Called
# by `make check` and `make ci`; LEAK_TMP and LEAK_ROOT point it elsewhere
# for its own tests.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=${LEAK_ROOT:-$(dirname "$here")}
tmp=${LEAK_TMP:-/tmp}
found=0

# report prints one leak and remembers that the check failed.
report() {
    printf 'leak: %s\n' "$1" >&2
    found=1
}

if command -v docker >/dev/null 2>&1; then
    for name in $(docker ps -a --format '{{.Names}}' --filter name=act-CI- 2>/dev/null); do
        report "container $name is still there (docker rm -f $name)"
    done
fi

for path in "$tmp"/oma-*; do
    [ -e "$path" ] || continue
    report "$path is in a RAM-backed temp directory; keep caches and scratch under build/"
done

# Running as root (inside a CI container), every file is root-owned by
# design, so only a normal user's checkout can be checked.
owned=""
if [ "$(id -u)" -ne 0 ]; then
    owned=$(find "$root" -path "$root/.git" -prune -o -user 0 -print 2>/dev/null | head -n 5)
fi
if [ -n "$owned" ]; then
    report "root-owned files in the checkout (run containers with -u \$(id -u):\$(id -g)): $(echo "$owned" | tr '\n' ' ')"
fi

exit "$found"
