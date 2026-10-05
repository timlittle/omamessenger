#!/usr/bin/env bash
set -Eeuo pipefail

test_root=/tmp/oma-e2e
export HOME="$test_root/home"
export XDG_CONFIG_HOME="$test_root/config"
export XDG_CACHE_HOME="$test_root/cache"
export XDG_RUNTIME_DIR=/tmp/r
export XDG_STATE_HOME="$test_root/state"
export WAYLAND_DISPLAY=wayland-0
export QT_QPA_PLATFORM=wayland
export LIBGL_ALWAYS_SOFTWARE=1
export QML_IMPORT_PATH="/app/tests/e2e/mocks${QML_IMPORT_PATH:+:$QML_IMPORT_PATH}"
export QML2_IMPORT_PATH="$QML_IMPORT_PATH"

mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$XDG_RUNTIME_DIR" "$XDG_STATE_HOME"
chmod 700 "$XDG_RUNTIME_DIR"

hyprland_pid=
helper_pid=
quickshell_pid=

cleanup() {
    result=$?
    trap - EXIT HUP INT TERM
    if [ -n "$quickshell_pid" ]; then kill "$quickshell_pid" 2>/dev/null || true; fi
    if [ -n "$helper_pid" ]; then kill "$helper_pid" 2>/dev/null || true; fi
    if [ -n "$hyprland_pid" ]; then kill "$hyprland_pid" 2>/dev/null || true; fi
    wait 2>/dev/null || true
    if [ "$result" -ne 0 ]; then
        printf '\n--- Hyprland log ---\n' >&2
        cat "$test_root/hyprland.log" >&2 2>/dev/null || true
        for crash_report in "$XDG_CACHE_HOME"/hyprland/hyprlandCrashReport*.txt; do
            [ -f "$crash_report" ] || continue
            printf '\n--- Hyprland crash report (%s) ---\n' "$crash_report" >&2
            cat "$crash_report" >&2
        done
        printf '\n--- Quickshell log ---\n' >&2
        cat "$test_root/quickshell.log" >&2 2>/dev/null || true
        printf '\n--- Helper log ---\n' >&2
        cat "$test_root/helper.log" >&2 2>/dev/null || true
    fi
    exit "$result"
}
trap cleanup EXIT HUP INT TERM

wait_until() {
    description=$1
    shift
    attempt=0
    while [ "$attempt" -lt 100 ]; do
        if "$@"; then return 0; fi
        sleep 0.1
        attempt=$((attempt + 1))
    done
    printf 'Timed out waiting for %s.\n' "$description" >&2
    return 1
}

has_window() {
    hyprctl -j clients | jq -e --arg title "$1" 'any(.[]; .title == $title)' >/dev/null
}

window_workspace() {
    hyprctl -j clients | jq -r --arg title "$1" 'first(.[] | select(.title == $title)).workspace.id'
}

helper_is_ready() {
    curl --fail --silent http://127.0.0.1:43821/api/v1/health >/dev/null
}

quickshell_ipc() {
    quickshell ipc --pid "$quickshell_pid" call "$@" >/dev/null 2>&1
}

no_app_window() {
    ! has_window OmaMessenger
}

printf 'Running pure keyboard state tests...\n'
node /app/tests/unit/keyboard.test.cjs

printf 'Starting a private headless Hyprland compositor...\n'
HYPRLAND_HEADLESS_ONLY=1 Hyprland --config /app/tests/e2e/hyprland.conf >"$test_root/hyprland.log" 2>&1 &
hyprland_pid=$!

attempt=0
while [ "$attempt" -lt 100 ]; do
    for socket_path in "$XDG_RUNTIME_DIR"/hypr/*/.socket.sock; do
        [ -S "$socket_path" ] || continue
        export HYPRLAND_INSTANCE_SIGNATURE
        HYPRLAND_INSTANCE_SIGNATURE=$(basename "$(dirname "$socket_path")")
        if hyprctl -j monitors >/dev/null 2>&1; then break 2; fi
    done
    sleep 0.1
    attempt=$((attempt + 1))
done
if [ -z "${HYPRLAND_INSTANCE_SIGNATURE:-}" ]; then
    printf 'Headless Hyprland did not become ready.\n' >&2
    exit 1
fi

printf 'Starting the bundled helper and verifying API persistence...\n'
start_helper() {
    XDG_CONFIG_HOME="$XDG_CONFIG_HOME" \
        OMA_DB="$XDG_CONFIG_HOME/omamessenger/messages.db" \
        OMA_PORT=43821 /app/bin/oma-messenger-service >"$test_root/helper.log" 2>&1 &
    helper_pid=$!
    wait_until 'helper health endpoint' helper_is_ready
}
start_helper

token_file="$XDG_CONFIG_HOME/omamessenger/api.token"
api_token=$(cat "$token_file")
curl --fail --silent --show-error -X POST http://127.0.0.1:43821/api/v1/accounts \
    -H 'Content-Type: application/json' -H "Authorization: Bearer $api_token" \
    -d '{"id":"e2e-account","service":"whatsapp","name":"E2E account"}' >/dev/null
curl --fail --silent --show-error http://127.0.0.1:43821/api/v1/accounts \
    -H "Authorization: Bearer $api_token" | jq -e 'any(.[]; .id == "e2e-account")' >/dev/null
kill "$helper_pid"
wait "$helper_pid" || true
helper_pid=
start_helper
curl --fail --silent --show-error http://127.0.0.1:43821/api/v1/accounts \
    -H "Authorization: Bearer $api_token" | jq -e 'any(.[]; .id == "e2e-account")' >/dev/null
printf 'Backend API and SQLite persistence checks passed.\n'

printf 'Starting the real Quickshell panel with mocked Omarchy theme/service modules...\n'
quickshell --path /app/tests/e2e/shell.qml >"$test_root/quickshell.log" 2>&1 &
quickshell_pid=$!
wait_until 'plugin IPC registration' quickshell_ipc io.github.omamessenger open
wait_until 'OmaMessenger normal toplevel' has_window OmaMessenger
app_workspace=$(window_workspace OmaMessenger)
if [ -z "$app_workspace" ] || [ "$app_workspace" = null ]; then
    printf 'OmaMessenger did not appear in Hyprland as a normal window.\n' >&2
    exit 1
fi
printf 'Normal Hyprland toplevel created on workspace %s.\n' "$app_workspace"

printf 'Checking focus isolation from other application windows...\n'
quickshell_ipc oma-messenger-e2e showFocusProbe
wait_until 'focus probe toplevel' has_window 'OmaMessenger Focus Probe'
active_title=$(hyprctl -j activewindow | jq -r '.title')
if [ "$active_title" != "OmaMessenger Focus Probe" ]; then
    printf 'Focus did not move to the other window.\n' >&2
    exit 1
fi
wtype -k Escape
if ! has_window OmaMessenger; then
    printf 'Escape in another window incorrectly closed OmaMessenger.\n' >&2
    exit 1
fi
quickshell_ipc oma-messenger-e2e hideFocusProbe

printf 'Checking workspace switching...\n'
hyprctl dispatch workspace +1 >/dev/null
active_workspace=$(hyprctl -j activeworkspace | jq -r '.id')
if [ "$active_workspace" = "$app_workspace" ] || ! has_window OmaMessenger; then
    printf 'OmaMessenger blocked or disappeared during workspace switching.\n' >&2
    exit 1
fi
hyprctl dispatch workspace "$app_workspace" >/dev/null
hyprctl dispatch focuswindow 'title:OmaMessenger' >/dev/null

printf 'Checking Escape and IPC dismissal/reopen...\n'
wtype -k Escape
wait_until 'Escape to hide OmaMessenger' no_app_window
quickshell_ipc io.github.omamessenger open
wait_until 'IPC to reopen OmaMessenger' has_window OmaMessenger

printf 'Checking the Hyprland close-window request...\n'
hyprctl dispatch closewindow 'title:OmaMessenger' >/dev/null
wait_until 'compositor close request to hide OmaMessenger' no_app_window

printf 'Docker E2E passed: API persistence, IPC, focus, Escape, workspace switching, and window close.\n'
