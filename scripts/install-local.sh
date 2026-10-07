#!/usr/bin/env bash
# Install this checkout as the Omarchy plugin, for trying changes locally.
# Called by `make install-local` (and `make validate` with --check).
#
# It stages only the files the plugin needs, validates that staged copy
# (the repository itself holds build symlinks the validator rejects),
# copies it into the plugin directory and enables it. Omarchy watches the
# plugin directory and reloads a plugin whose files change, clearing Qt's
# QML cache, so the running shell picks up the new code by itself. Do not
# also restart the shell: a restart on top of that reload destroys and
# recreates the plugin's service at once, which has crashed Quickshell.
# With --check it stops after validating and installs nothing; with
# --restart it restarts the shell anyway, for when a reload did not take.
set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
plugin_id=io.github.omamessenger
expected_dir="${HOME}/.config/omarchy/plugins/${plugin_id}"
check_only=false
restart=false

case "${1:-}" in
    --check) check_only=true; shift ;;
    --restart) restart=true; shift ;;
esac

target_dir=${1:-$expected_dir}
if [[ "$target_dir" != "$expected_dir" ]]; then
    printf 'Refusing unexpected plugin install path: %s\n' "$target_dir" >&2
    exit 1
fi

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

# The validator expects the folder to be named after the plugin id.
staging_dir="$tmp_dir/$plugin_id"
mkdir -p "$staging_dir/bin/dev" "$staging_dir/scripts"
cp "$repo_root/manifest.json" "$repo_root/LICENSE" "$repo_root/helper-version" "$staging_dir/"
cp -r "$repo_root/ui" "$staging_dir/ui"
cp "$repo_root/bin/oma-messenger-service" "$staging_dir/bin/"
cp "$repo_root/scripts/install-helper.sh" "$staging_dir/scripts/"
if [[ -x "$repo_root/bin/dev/oma-messenger-service" ]]; then
    cp "$repo_root/bin/dev/oma-messenger-service" "$staging_dir/bin/dev/"
fi

"${OMARCHY:-omarchy}" plugin validate "$staging_dir"
if [[ "$check_only" == true ]]; then
    printf 'Plugin files are valid\n'
    exit 0
fi

mkdir -p "$target_dir"
"${RSYNC:-rsync}" -a --delete "$staging_dir/" "$target_dir/"
"${OMARCHY:-omarchy}" plugin enable "$plugin_id"
if [[ "$restart" == false ]]; then
    printf 'Installed %s from %s; Omarchy reloads it in a moment\n' "$plugin_id" "$repo_root"
    exit 0
fi

"${OMARCHY_RESTART_SHELL:-omarchy-restart-shell}"

# Wait for the restarted shell to answer, so a summon right after this
# script reaches the new plugin. listPlugins only reads: rescanPlugins
# would reload the plugin while the shell is still starting.
for _ in $(seq 1 50); do
    if "${OMARCHY_SHELL:-omarchy-shell}" shell listPlugins >/dev/null 2>&1; then
        printf 'Installed %s from %s and restarted the shell\n' "$plugin_id" "$repo_root"
        exit 0
    fi
    sleep 0.2
done
printf 'Installed %s, but the shell did not come back within 10 seconds\n' "$plugin_id" >&2
exit 1
