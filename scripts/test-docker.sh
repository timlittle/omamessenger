#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_context=$(mktemp -d)
trap 'rm -rf "$test_context"' EXIT

cp "$repo_root/Panel.qml" "$repo_root/keyboard.js" "$test_context/"
cp -a "$repo_root/bin" "$test_context/"
mkdir -p "$test_context/tests"
cp -a "$repo_root/tests/e2e" "$test_context/tests/"
cp -a "$repo_root/tests/unit" "$test_context/tests/"

docker build --tag oma-messenger-e2e:local --file "$test_context/tests/e2e/Dockerfile" "$test_context"
docker run --rm --network none oma-messenger-e2e:local
