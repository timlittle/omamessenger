#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
plugin_id=io.github.omamessenger
expected_dir="${HOME}/.config/omarchy/plugins/${plugin_id}"
target_dir=${1:-$expected_dir}

if [[ "$target_dir" != "$expected_dir" ]]; then
    printf 'Refusing unexpected plugin install path: %s\n' "$target_dir" >&2
    exit 1
fi

staging_dir=$(mktemp -d)
trap 'rm -rf "$staging_dir"' EXIT
mkdir -p "$staging_dir/bin" "$target_dir"
cp "$repo_root/manifest.json" "$repo_root/Panel.qml" "$repo_root/Service.qml" "$repo_root/LICENSE" "$staging_dir/"
cp "$repo_root/bin/oma-messenger-service" "$repo_root"/bin/oma-messenger-service-linux-* "$staging_dir/bin/"

"${OMARCHY:-omarchy}" plugin validate "$repo_root"
"${RSYNC:-rsync}" -a "$staging_dir/" "$target_dir/"
"${OMARCHY:-omarchy}" plugin validate "$target_dir"
"${OMARCHY_SHELL:-omarchy-shell}" shell rescanPlugins
"${OMARCHY:-omarchy}" plugin enable "$plugin_id"
printf 'Installed and enabled %s from %s\n' "$plugin_id" "$repo_root"
