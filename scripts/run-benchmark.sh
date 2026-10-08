#!/usr/bin/env bash
# Runs the full OmaMessenger benchmark against the user's real desktop and
# writes the result to docs/BENCHMARK.md. Called by `make benchmark`; not
# part of `make check`, since it restarts omarchy-shell and launches
# Telegram Desktop and Chromium, so a human runs it on purpose, never CI
# or an agent.
#
# It uses scripts/benchmark.sh to sample the shell and the OmaMessenger
# helper with the window closed and open, Telegram Desktop, and WhatsApp
# Web in a disposable Chromium profile (or the user's own default profile,
# with BENCH_WHATSAPP_PROFILE=1). Settle time, sample count and the
# interval between samples come from scripts/benchmark.sh's own
# BENCH_SETTLE, BENCH_SAMPLES and BENCH_INTERVAL.
#
# On any failure it stops only what this run started (Telegram Desktop,
# its own Chromium instance, its temporary profile directory) and leaves
# omarchy-shell running, never restarting on top of a failure.
set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
plugin_id=io.github.omamessenger
plugin_dir="${HOME}/.config/omarchy/plugins/${plugin_id}"
telegram_bin="${TELEGRAM:-Telegram}"
chromium_bin="${CHROMIUM:-chromium}"
sampler="${BENCH_SAMPLER:-$repo_root/scripts/benchmark.sh}"
settle="${BENCH_SETTLE:-20}"
samples="${BENCH_SAMPLES:-3}"
interval="${BENCH_INTERVAL:-10}"
out_file="${BENCH_OUTPUT:-$repo_root/docs/BENCHMARK.md}"

telegram_pid=""
whatsapp_pids=""
whatsapp_launch_pid=""
whatsapp_dir=""

# log prints a progress line to stderr, so stdout carries only the final
# report.
log() {
    printf '==> %s\n' "$1" >&2
}

# wait_gone polls, the way install-local.sh waits for the shell, until
# none of the given pids answer kill -0, or gives up after 10 seconds.
# A pid this script backgrounded directly should be reaped with wait
# first; this is for the rest of a process tree, which are not this
# script's children and so cannot be waited on.
wait_gone() {
    for _ in $(seq 1 50); do
        alive=0
        for pid in "$@"; do
            if kill -0 "$pid" 2>/dev/null; then
                alive=1
            fi
        done
        if [ "$alive" -eq 0 ]; then
            return 0
        fi
        sleep 0.2
    done
    log "Warning: a process from this run did not exit within 10 seconds"
}

# stop_telegram terminates Telegram Desktop if sample_telegram's launch
# is still running, and clears telegram_pid so a second call (from the
# normal flow, then again from cleanup on an earlier failure) is a no-op.
stop_telegram() {
    if [ -z "$telegram_pid" ]; then
        return 0
    fi
    kill -TERM "$telegram_pid" 2>/dev/null || true
    wait "$telegram_pid" 2>/dev/null || true
    telegram_pid=""
}

# stop_whatsapp terminates every pid sample_whatsapp found, waits for the
# one it backgrounded directly (the disposable Chromium instance; the
# default-profile instance is not this script's child), removes the
# temporary profile directory if there was one, and clears both so a
# second call is a no-op.
stop_whatsapp() {
    if [ -n "$whatsapp_pids" ]; then
        IFS=',' read -r -a pids <<< "$whatsapp_pids"
        kill -TERM "${pids[@]}" 2>/dev/null || true
        if [ -n "$whatsapp_launch_pid" ]; then
            wait "$whatsapp_launch_pid" 2>/dev/null || true
            whatsapp_launch_pid=""
        fi
        wait_gone "${pids[@]}"
        whatsapp_pids=""
    fi
    if [ -n "$whatsapp_dir" ]; then
        rm -rf "$whatsapp_dir"
        whatsapp_dir=""
    fi
}

# cleanup stops whatever this run started that is still alive and removes
# its temporary Chromium profile. It runs on every exit, including a
# failure, and it never touches omarchy-shell: a half-finished benchmark
# leaves the shell running, not restarted again on top of itself.
cleanup() {
    if [ -n "$whatsapp_pids" ] || [ -n "$whatsapp_dir" ]; then
        log "Cleaning up: stopping the benchmark's Chromium instance"
        stop_whatsapp
    fi
    if [ -n "$telegram_pid" ]; then
        log "Cleaning up: stopping Telegram Desktop"
        stop_telegram
    fi
}
trap cleanup EXIT

# pid_list prints the comma-joined pids pgrep finds for the given
# arguments, or fails with a clear message when none are running.
pid_list() {
    desc="$1"
    shift
    out=$(pgrep "$@" | tr '\n' ',' | sed 's/,$//') || true
    if [ -z "$out" ]; then
        printf 'run-benchmark: no running process found for %s\n' "$desc" >&2
        exit 1
    fi
    printf '%s\n' "$out"
}

# process_tree prints pid $1 plus every descendant it can reach through
# pgrep -P, comma separated, so a launched app's own helper processes are
# counted alongside it.
process_tree() {
    all="$1"
    frontier="$1"
    while true; do
        next=$(pgrep -P "$frontier" | tr '\n' ',' | sed 's/,$//') || true
        if [ -z "$next" ]; then
            break
        fi
        all="$all,$next"
        frontier="$next"
    done
    printf '%s\n' "$all"
}

# preflight refuses to start while a heavy application or Telegram Desktop
# is running, since either would distort the measurements, or a
# prerequisite is missing.
preflight() {
    log "Checking for heavy applications and prerequisites"
    heavy=""
    if pgrep -x chromium >/dev/null 2>&1; then
        heavy="${heavy}  - Chromium (chromium)\n"
    fi
    if pgrep -x bambustu_main >/dev/null 2>&1; then
        heavy="${heavy}  - Bambu Studio (bambustu_main)\n"
    fi
    # A second Telegram Desktop hands over to the running one and exits,
    # leaving nothing of this run's own to measure.
    if pgrep -x "$(basename "$telegram_bin")" >/dev/null 2>&1; then
        heavy="${heavy}  - Telegram Desktop ($(basename "$telegram_bin"))\n"
    fi
    if [ -n "$heavy" ]; then
        printf 'run-benchmark: close these first, then run again:\n' >&2
        printf '%b' "$heavy" >&2
        exit 1
    fi

    if [ ! -d "$plugin_dir" ]; then
        printf 'run-benchmark: OmaMessenger is not installed at %s; run "make install-local" first\n' "$plugin_dir" >&2
        exit 1
    fi
    if ! command -v "$telegram_bin" >/dev/null 2>&1; then
        printf 'run-benchmark: Telegram Desktop (%s) is not installed\n' "$telegram_bin" >&2
        exit 1
    fi
    if ! command -v "$chromium_bin" >/dev/null 2>&1; then
        printf 'run-benchmark: Chromium (%s) is not installed\n' "$chromium_bin" >&2
        exit 1
    fi
}

# baseline restarts omarchy-shell for a clean reading, waits for it the
# way install-local.sh does (polling the read-only listPlugins, never
# rescanPlugins), and makes sure the OmaMessenger window starts closed.
baseline() {
    log "Restarting omarchy-shell for a clean baseline"
    "${OMARCHY_RESTART_SHELL:-omarchy-restart-shell}"
    i=0
    until "${OMARCHY_SHELL:-omarchy-shell}" shell listPlugins >/dev/null 2>&1; do
        i=$((i + 1))
        if [ "$i" -ge 50 ]; then
            printf 'run-benchmark: the shell did not come back within 10 seconds\n' >&2
            exit 1
        fi
        sleep 0.2
    done
    "${OMARCHY_SHELL:-omarchy-shell}" -q shell hide "$plugin_id"
}

# sample_baseline finds the shell and helper pids and samples them with
# the OmaMessenger window closed, into closed_table.
sample_baseline() {
    log "Sampling the shell and the helper, window closed"
    shell_pid=$(pid_list "quickshell" -x quickshell)
    helper_pid=$(pid_list "the OmaMessenger helper" -f 'oma-messenger-service$')
    closed_table=$("$sampler" "shell=$shell_pid" "helper=$helper_pid")
}

# sample_open opens the OmaMessenger window and samples the same shell
# and helper pids again, into open_table, then closes the window.
sample_open() {
    log "Opening the OmaMessenger window"
    "${OMARCHY_SHELL:-omarchy-shell}" shell summon "$plugin_id" '{}' >/dev/null
    log "Sampling the shell and the helper, window open"
    open_table=$("$sampler" "shell=$shell_pid" "helper=$helper_pid")
    "${OMARCHY_SHELL:-omarchy-shell}" -q shell hide "$plugin_id"
}

# sample_telegram launches Telegram Desktop, samples its process tree
# into telegram_table, then closes it gracefully.
sample_telegram() {
    log "Launching Telegram Desktop"
    "$telegram_bin" >/dev/null 2>&1 &
    telegram_pid=$!
    sleep "$settle"
    telegram_tree=$(process_tree "$telegram_pid")
    log "Sampling Telegram Desktop"
    telegram_table=$("$sampler" "telegram=$telegram_tree")

    log "Closing Telegram Desktop"
    stop_telegram
}

# sample_whatsapp opens WhatsApp Web, logged out in a disposable Chromium
# profile unless BENCH_WHATSAPP_PROFILE=1 asks for the user's own default
# profile, samples the whole Chromium process tree into whatsapp_table
# and sets whatsapp_note to describe which mode ran, then closes it.
sample_whatsapp() {
    if [ "${BENCH_WHATSAPP_PROFILE:-0}" = "1" ]; then
        log "Launching WhatsApp Web in your default Chromium profile"
        "${OMARCHY_LAUNCH_WEBAPP:-omarchy-launch-webapp}" https://web.whatsapp.com/ >/dev/null 2>&1 &
        sleep "$settle"
        whatsapp_pids=$(pid_list "chromium" -x "$chromium_bin")
        whatsapp_note="your logged-in default profile (BENCH_WHATSAPP_PROFILE=1)"
    else
        log "Launching a disposable Chromium instance for WhatsApp Web (signed out)"
        whatsapp_dir=$(mktemp -d)
        "$chromium_bin" "--user-data-dir=$whatsapp_dir" --no-first-run --app=https://web.whatsapp.com/ >/dev/null 2>&1 &
        whatsapp_launch_pid=$!
        sleep "$settle"
        whatsapp_pids=$(process_tree "$whatsapp_launch_pid")
        whatsapp_note="signed out, in a disposable profile (set BENCH_WHATSAPP_PROFILE=1 to measure your logged-in profile instead)"
    fi

    log "Sampling WhatsApp Web"
    whatsapp_table=$("$sampler" "whatsapp=$whatsapp_pids")

    log "Closing WhatsApp Web"
    stop_whatsapp
}

# cell prints one column (3=RSS MiB, 4=own memory MiB, 5=CPU%, 6=context
# switches/s) of the row labeled $2 in a benchmark.sh table captured as
# $1, or fails if that label's row is missing.
cell() {
    printf '%s\n' "$1" | awk -F'|' -v label="$2" -v col="$3" '
        { l = $2; gsub(/^[ \t]+|[ \t]+$/, "", l) }
        l == label { v = $col; gsub(/^[ \t]+|[ \t]+$/, "", v); print v; found = 1 }
        END { if (!found) { print "run-benchmark: no row for " label > "/dev/stderr"; exit 1 } }'
}

# arith prints $1 $2 $3 (a plus or minus sign) formatted to $4 decimal
# places, for combining two sampled columns into a derived row.
arith() {
    awk -v a="$1" -v b="$2" -v op="$3" -v p="$4" 'BEGIN {
        r = (op == "+") ? a + b : a - b
        printf "%." p "f", r
    }'
}

# omarchy_version prints the installed Omarchy version, or "unknown" when
# the command is missing.
omarchy_version() {
    "${OMARCHY_VERSION:-omarchy-version}" 2>/dev/null || echo "unknown"
}

# qt_version prints the installed qt6-base package version, or "unknown"
# when pacman cannot report it.
qt_version() {
    pacman -Q qt6-base 2>/dev/null | awk '{ print $2 }' || echo "unknown"
}

# report_row prints one Markdown table row for the five-column report.
report_row() {
    printf '| %s | %s | %s | %s | %s |\n' "$1" "$2" "$3" "$4" "$5"
}

# generate_report combines the four captured tables into docs/BENCHMARK.md
# (closed/open shell, the window's own cost, the helper, OmaMessenger's
# total, Telegram Desktop and WhatsApp Web) and prints it.
generate_report() {
    rss_closed=$(cell "$closed_table" shell 3); own_closed=$(cell "$closed_table" shell 4)
    cpu_closed=$(cell "$closed_table" shell 5); ctx_closed=$(cell "$closed_table" shell 6)

    rss_open=$(cell "$open_table" shell 3); own_open=$(cell "$open_table" shell 4)
    cpu_open=$(cell "$open_table" shell 5); ctx_open=$(cell "$open_table" shell 6)

    rss_helper=$(cell "$closed_table" helper 3); own_helper=$(cell "$closed_table" helper 4)
    cpu_helper=$(cell "$closed_table" helper 5); ctx_helper=$(cell "$closed_table" helper 6)

    rss_window=$(arith "$rss_open" "$rss_closed" - 1); own_window=$(arith "$own_open" "$own_closed" - 1)
    cpu_window=$(arith "$cpu_open" "$cpu_closed" - 2); ctx_window=$(arith "$ctx_open" "$ctx_closed" - 1)

    rss_total=$(arith "$rss_window" "$rss_helper" + 1); own_total=$(arith "$own_window" "$own_helper" + 1)
    cpu_total=$(arith "$cpu_window" "$cpu_helper" + 2); ctx_total=$(arith "$ctx_window" "$ctx_helper" + 1)

    rss_tg=$(cell "$telegram_table" telegram 3); own_tg=$(cell "$telegram_table" telegram 4)
    cpu_tg=$(cell "$telegram_table" telegram 5); ctx_tg=$(cell "$telegram_table" telegram 6)

    rss_wa=$(cell "$whatsapp_table" whatsapp 3); own_wa=$(cell "$whatsapp_table" whatsapp 4)
    cpu_wa=$(cell "$whatsapp_table" whatsapp 5); ctx_wa=$(cell "$whatsapp_table" whatsapp 6)

    {
        cat <<EOF
# OmaMessenger benchmark

Memory and CPU use of OmaMessenger next to Telegram Desktop and WhatsApp
Web, measured on this machine. Rerun it with:

    make benchmark

Close Chromium and any other heavy application first: the script refuses
to start otherwise, since it also restarts omarchy-shell and opens
Telegram Desktop and a Chromium window of its own.

Method: scripts/run-benchmark.sh drives scripts/benchmark.sh, which reads
/proc/<pid>/status and /proc/<pid>/stat after a settle period and reports
the median of several samples. "Own memory" is RssAnon plus swap: the
process's own anonymous pages, leaving out memory it shares with other
processes. PSS would be fairer for shared libraries, but reading another
process's /proc/<pid>/smaps_rollup needs ptrace access that Yama's
default scope only grants to its parent.

## Results

- Date: $(date -u +%Y-%m-%dT%H:%MZ)
- Kernel: $(uname -r)
- Omarchy: $(omarchy_version)
- Qt: $(qt_version)
- Samples: median of $samples, ${interval}s apart, after a ${settle}s settle
- WhatsApp Web: $whatsapp_note

| Process | RSS (MiB) | Own memory, RAM + swap (MiB) | CPU (%) | Context switches/s |
| --- | ---: | ---: | ---: | ---: |
EOF
        report_row "Shell, window closed" "$rss_closed" "$own_closed" "$cpu_closed" "$ctx_closed"
        report_row "Shell, window open" "$rss_open" "$own_open" "$cpu_open" "$ctx_open"
        report_row "OmaMessenger window (open minus closed)" "$rss_window" "$own_window" "$cpu_window" "$ctx_window"
        report_row "OmaMessenger helper" "$rss_helper" "$own_helper" "$cpu_helper" "$ctx_helper"
        report_row "OmaMessenger total (window plus helper)" "$rss_total" "$own_total" "$cpu_total" "$ctx_total"
        report_row "Telegram Desktop" "$rss_tg" "$own_tg" "$cpu_tg" "$ctx_tg"
        report_row "WhatsApp Web" "$rss_wa" "$own_wa" "$cpu_wa" "$ctx_wa"
    } > "$out_file"
    cat "$out_file"
}

main() {
    preflight
    baseline
    sample_baseline
    sample_open
    sample_telegram
    sample_whatsapp
    generate_report
    log "Done. Results written to $out_file"
}

main
