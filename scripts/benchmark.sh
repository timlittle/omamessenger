#!/usr/bin/env sh
# Samples the memory and CPU use of running processes, for docs/BENCHMARK.md.
# Run by hand; nothing else calls it. It only reads /proc, so it never
# changes the processes it measures.
#
#   scripts/benchmark.sh shell=1234 helper=5678 telegram=91011
#
# Each argument is label=pid[,pid...]; a label's pids are added together
# (a browser tab and its helpers, say). After BENCH_SETTLE seconds (20) it
# takes BENCH_SAMPLES samples (3), BENCH_INTERVAL seconds apart (10), and
# prints one Markdown table row per label holding the median of each
# column. PSS would be fairer than RSS for shared libraries, but
# /proc/<pid>/smaps_rollup needs ptrace access to the process, which
# Yama's default scope only grants to its parent.
set -eu

settle="${BENCH_SETTLE:-20}"
samples="${BENCH_SAMPLES:-3}"
interval="${BENCH_INTERVAL:-10}"
ticks="$(getconf CLK_TCK)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# status_kb prints a /proc/<pid>/status field in kB, or 0 when absent.
status_kb() {
    awk -v key="$2:" '$1 == key { print $2; found = 1 } END { if (!found) print 0 }' "/proc/$1/status"
}

# cpu_ticks prints the user plus system CPU time a process has used, in
# clock ticks. The command name in field 2 may hold spaces, so fields are
# counted from the closing parenthesis.
cpu_ticks() {
    sed 's/.*) //' "/proc/$1/stat" | awk '{ print $12 + $13 }'
}

# switches prints a process's voluntary plus involuntary context switches.
switches() {
    awk '/^(voluntary|nonvoluntary)_ctxt_switches:/ { n += $2 } END { print n + 0 }' "/proc/$1/status"
}

# totals prints "rss anon swap ticks switches" summed over a pid list.
totals() {
    rss=0 anon=0 swap=0 cpu=0 ctx=0
    for pid in $(echo "$1" | tr ',' ' '); do
        [ -r "/proc/$pid/status" ] || { echo "benchmark: pid $pid is not running" >&2; exit 1; }
        rss=$((rss + $(status_kb "$pid" VmRSS)))
        anon=$((anon + $(status_kb "$pid" RssAnon)))
        swap=$((swap + $(status_kb "$pid" VmSwap)))
        cpu=$((cpu + $(cpu_ticks "$pid")))
        ctx=$((ctx + $(switches "$pid")))
    done
    echo "$rss $anon $swap $cpu $ctx"
}

# median prints the middle value of the numbers on stdin.
median() {
    sort -n | awk '{ v[NR] = $1 } END { print v[int((NR + 1) / 2)] }'
}

[ "$#" -gt 0 ] || { echo "usage: benchmark.sh label=pid[,pid...] ..." >&2; exit 2; }

sleep "$settle"
for arg in "$@"; do
    label="${arg%%=*}"
    totals "${arg#*=}" > "$work/$label.prev"
done

i=0
while [ "$i" -lt "$samples" ]; do
    sleep "$interval"
    for arg in "$@"; do
        label="${arg%%=*}"
        now="$(totals "${arg#*=}")"
        # Memory is read at the sample; CPU and switches are the change
        # since the previous sample, as a rate.
        echo "$now" "$(cat "$work/$label.prev")" | awk -v t="$ticks" -v s="$interval" \
            '{ printf "%d %d %d %.2f %.1f\n", $1, $2, $3, ($4 - $9) * 100 / t / s, ($5 - $10) / s }' \
            >> "$work/$label.samples"
        echo "$now" > "$work/$label.prev"
    done
    i=$((i + 1))
done

echo "| Process | RSS (MiB) | Own memory, RAM + swap (MiB) | CPU (%) | Context switches/s |"
echo "| --- | ---: | ---: | ---: | ---: |"
for arg in "$@"; do
    label="${arg%%=*}"
    f="$work/$label.samples"
    rss="$(cut -d' ' -f1 "$f" | median)"
    own="$(awk '{ print $2 + $3 }' "$f" | median)"
    cpu="$(cut -d' ' -f4 "$f" | median)"
    ctx="$(cut -d' ' -f5 "$f" | median)"
    awk -v l="$label" -v r="$rss" -v o="$own" -v c="$cpu" -v x="$ctx" \
        'BEGIN { printf "| %s | %.1f | %.1f | %s | %s |\n", l, r / 1024, o / 1024, c, x }'
done
