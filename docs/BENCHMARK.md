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

- Date: 2026-10-08T20:39Z
- Kernel: 7.2.5-3-omarchy
- Omarchy: 4.0.4-1
- Qt: 6.11.2-3
- Samples: median of 3, 10s apart, after a 20s settle
- WhatsApp Web: signed out, in a disposable profile (set BENCH_WHATSAPP_PROFILE=1 to measure your logged-in profile instead)

| Process | RSS (MiB) | Own memory, RAM + swap (MiB) | CPU (%) | Context switches/s |
| --- | ---: | ---: | ---: | ---: |
| Shell, window closed | 427.3 | 265.7 | 0.00 | 10.9 |
| Shell, window open | 500.6 | 320.1 | 0.10 | 17.1 |
| OmaMessenger window (open minus closed) | 73.3 | 54.4 | 0.10 | 6.2 |
| OmaMessenger helper | 49.3 | 26.3 | 0.00 | 0.0 |
| OmaMessenger total (window plus helper) | 122.6 | 80.7 | 0.10 | 6.2 |
| Telegram Desktop | 660.1 | 360.5 | 0.00 | 2.0 |
| WhatsApp Web | 1287.1 | 411.3 | 1.30 | 31.8 |
