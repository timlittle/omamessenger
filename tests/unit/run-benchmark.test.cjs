// Tests for scripts/run-benchmark.sh. Every external command it touches
// (omarchy-restart-shell, omarchy-shell, pgrep, Telegram, chromium,
// omarchy-launch-webapp, sleep and the sampler) is a stub on PATH or an
// overridden path, so this never restarts the real shell, opens a real
// app, or writes the real docs/BENCHMARK.md.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const repo = path.resolve(__dirname, '../..');
const realSleep = spawnSync('sh', ['-c', 'command -v sleep'], { encoding: 'utf8' }).stdout.trim();

// writeStub creates an executable shell script at file with the given body.
function writeStub(file, body) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, body);
  fs.chmodSync(file, 0o755);
}

// sandbox builds a temporary HOME, plugin directory, pgrep fixtures and a
// PATH of stub commands, and returns everything a test needs to run
// scripts/run-benchmark.sh in isolation. pluginInstalled controls whether
// the plugin directory preflight expects is created.
function sandbox(t, { pluginInstalled = true } = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'oma-benchmark-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const home = path.join(root, 'home');
  const bin = path.join(root, 'bin');
  const fixtures = path.join(root, 'fixtures');
  const callLog = path.join(root, 'calls.log');
  const samplerCalls = path.join(root, 'sampler-calls');
  fs.mkdirSync(bin, { recursive: true });
  fs.mkdirSync(fixtures, { recursive: true });
  fs.writeFileSync(callLog, '');

  // The shell and the OmaMessenger helper are always running, with no
  // need for real processes behind them: nothing in the script signals
  // or waits on these two pids, only the stubbed sampler reads them, and
  // it ignores their value.
  fs.writeFileSync(path.join(fixtures, 'x_quickshell'), '11111\n');
  fs.writeFileSync(path.join(fixtures, 'f_helper'), '22222\n');

  if (pluginInstalled) {
    fs.mkdirSync(path.join(home, '.config/omarchy/plugins/io.github.omamessenger'), { recursive: true });
  } else {
    fs.mkdirSync(home, { recursive: true });
  }

  // Answers pgrep with fixture files instead of the real process table,
  // so the test never depends on what else is running on this machine.
  writeStub(path.join(bin, 'pgrep'), `#!/bin/sh
case "$1" in
  -x) key="x_$2" ;;
  -f) key="f_helper" ;;
  -P) key="p_$(printf '%s' "$2" | tr ',' '_')" ;;
  *) echo "fake pgrep: unsupported arguments: $*" >&2; exit 2 ;;
esac
file="$PGREP_FIXTURES/$key"
if [ -s "$file" ]; then
  cat "$file"
  exit 0
fi
exit 1
`);

  // A no-op sleep, so every settle and poll wait in the script and in a
  // stub finishes instantly. Stubs that need a real, long-lived process
  // behind a fake pid call $REAL_SLEEP instead.
  writeStub(path.join(bin, 'sleep'), '#!/bin/sh\nexit 0\n');

  writeStub(path.join(bin, 'omarchy-restart-shell'), `#!/bin/sh
echo "omarchy-restart-shell $*" >> "$CALL_LOG"
exit 0
`);

  writeStub(path.join(bin, 'omarchy-shell'), `#!/bin/sh
echo "omarchy-shell $*" >> "$CALL_LOG"
if [ "$1" = "-q" ]; then shift; fi
if [ "$1" = "shell" ] && [ "$2" = "listPlugins" ]; then
  [ "\${FAIL_LISTPLUGINS:-0}" != "1" ] || exit 1
  echo "[]"
fi
exit 0
`);

  // Simulates omarchy-launch-webapp handing a URL to the default browser:
  // it registers a running "chromium" backed by a real sleep, then exits,
  // the way uwsm-app detaches the real browser from its own process tree.
  writeStub(path.join(bin, 'omarchy-launch-webapp'), `#!/bin/sh
echo "omarchy-launch-webapp $*" >> "$CALL_LOG"
"$REAL_SLEEP" 300 &
echo "$!" > "$PGREP_FIXTURES/x_chromium"
exit 0
`);

  // Simulates Telegram Desktop's main process. The real script only
  // signals this one pid (see run-benchmark.sh), so the stub has no
  // child of its own left orphaned when that pid is killed.
  writeStub(path.join(bin, 'Telegram'), `#!/bin/sh
echo "Telegram $*" >> "$CALL_LOG"
echo "$$" > "$TELEGRAM_PID_FILE"
exec "$REAL_SLEEP" 300
`);

  // Simulates Chromium spawning two child processes (a renderer and a
  // GPU process, say) under its own pid.
  writeStub(path.join(bin, 'chromium'), `#!/bin/sh
echo "chromium $*" >> "$CALL_LOG"
"$REAL_SLEEP" 300 &
c1=$!
"$REAL_SLEEP" 300 &
c2=$!
printf '%s\\n%s\\n' "$c1" "$c2" > "$PGREP_FIXTURES/p_$$"
echo "$$" > "$CHROMIUM_PID_FILE"
exec "$REAL_SLEEP" 300
`);

  // Stands in for scripts/benchmark.sh: prints a canned table instead of
  // reading /proc, and can fail on a chosen call to test cleanup on a
  // mid-run failure. Its first "shell" row differs from its second, so a
  // test can check the window-open-versus-closed delta.
  writeStub(path.join(root, 'sampler.sh'), `#!/bin/sh
n=$(( $(cat "$SAMPLER_CALLS" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$SAMPLER_CALLS"
echo "sampler $n: $*" >> "$CALL_LOG"
if [ "$n" = "\${FAIL_SAMPLER_CALL:-0}" ]; then
  echo "sampler: forced failure for test" >&2
  exit 1
fi
echo "| Process | RSS (MiB) | Own memory, RAM + swap (MiB) | CPU (%) | Context switches/s |"
echo "| --- | ---: | ---: | ---: | ---: |"
for arg in "$@"; do
  label=\${arg%%=*}
  case "$label" in
    shell)
      if [ "$n" = "1" ]; then echo "| shell | 100.0 | 50.0 | 1.00 | 2.0 |"
      else echo "| shell | 120.0 | 60.0 | 1.50 | 3.0 |"; fi ;;
    helper) echo "| helper | 10.0 | 5.0 | 0.10 | 0.5 |" ;;
    telegram) echo "| telegram | 200.0 | 150.0 | 2.00 | 4.0 |" ;;
    whatsapp) echo "| whatsapp | 300.0 | 250.0 | 3.00 | 5.0 |" ;;
  esac
done
`);

  const env = (extra) => ({
    ...process.env,
    HOME: home,
    PATH: `${bin}:${process.env.PATH}`,
    PGREP_FIXTURES: fixtures,
    CALL_LOG: callLog,
    SAMPLER_CALLS: samplerCalls,
    BENCH_SAMPLER: path.join(root, 'sampler.sh'),
    BENCH_OUTPUT: path.join(root, 'BENCHMARK.md'),
    REAL_SLEEP: realSleep,
    TELEGRAM_PID_FILE: path.join(root, 'telegram.pid'),
    CHROMIUM_PID_FILE: path.join(root, 'chromium.pid'),
    ...extra,
  });

  const run = (extra) => spawnSync(path.join(repo, 'scripts/run-benchmark.sh'), [], { encoding: 'utf8', env: env(extra) });

  // seedFixture pre-creates a pgrep answer, as if that process were
  // already running before the script starts.
  const seedFixture = (key, value) => fs.writeFileSync(path.join(fixtures, key), `${value}\n`);

  // readPid reads back a pid a stub recorded through one of the *_PID_FILE
  // env vars.
  const readPid = (file) => Number(fs.readFileSync(file, 'utf8').trim());

  // alive reports whether a pid is still running.
  const alive = (pid) => {
    try {
      process.kill(pid, 0);
      return true;
    } catch {
      return false;
    }
  };

  return { root, run, env, seedFixture, callLog, outFile: path.join(root, 'BENCHMARK.md'), readPid, alive };
}

test('runs the full benchmark and writes docs/BENCHMARK.md', (t) => {
  const s = sandbox(t);
  const result = s.run();
  assert.equal(result.status, 0, result.stderr);

  const calls = fs.readFileSync(s.callLog, 'utf8').trim().split('\n');
  assert.match(calls[0], /^omarchy-restart-shell/);
  assert.ok(calls.some((l) => /^omarchy-shell shell listPlugins/.test(l)));
  assert.ok(calls.some((l) => /^omarchy-shell -q shell hide io\.github\.omamessenger/.test(l)));
  assert.ok(calls.some((l) => /^omarchy-shell shell summon io\.github\.omamessenger \{\}/.test(l)));
  assert.ok(calls.some((l) => /^Telegram/.test(l)));
  assert.ok(calls.some((l) => /^chromium/.test(l)));
  assert.equal(calls.filter((l) => l.startsWith('sampler')).length, 4);

  // Everything this run started is gone once it finishes.
  assert.equal(s.alive(s.readPid(path.join(s.root, 'telegram.pid'))), false);
  assert.equal(s.alive(s.readPid(path.join(s.root, 'chromium.pid'))), false);

  const report = fs.readFileSync(s.outFile, 'utf8');
  assert.equal(result.stdout.trim(), report.trim());
  assert.match(report, /^# OmaMessenger benchmark$/m);
  assert.match(report, /- Date: \d{4}-\d{2}-\d{2}T\d{2}:\d{2}Z/);
  assert.match(report, /- Kernel: /);
  assert.match(report, /- Omarchy: /);
  assert.match(report, /- Qt: /);
  assert.match(report, /- WhatsApp Web: signed out, in a disposable profile/);
  assert.match(report, /\| Shell, window closed \| 100\.0 \| 50\.0 \| 1\.00 \| 2\.0 \|/);
  assert.match(report, /\| Shell, window open \| 120\.0 \| 60\.0 \| 1\.50 \| 3\.0 \|/);
  assert.match(report, /\| OmaMessenger window \(open minus closed\) \| 20\.0 \| 10\.0 \| 0\.50 \| 1\.0 \|/);
  assert.match(report, /\| OmaMessenger helper \| 10\.0 \| 5\.0 \| 0\.10 \| 0\.5 \|/);
  assert.match(report, /\| OmaMessenger total \(window plus helper\) \| 30\.0 \| 15\.0 \| 0\.60 \| 1\.5 \|/);
  assert.match(report, /\| Telegram Desktop \| 200\.0 \| 150\.0 \| 2\.00 \| 4\.0 \|/);
  assert.match(report, /\| WhatsApp Web \| 300\.0 \| 250\.0 \| 3\.00 \| 5\.0 \|/);
});

test('measures WhatsApp Web in the default profile when BENCH_WHATSAPP_PROFILE=1', (t) => {
  const s = sandbox(t);
  const result = s.run({ BENCH_WHATSAPP_PROFILE: '1' });
  assert.equal(result.status, 0, result.stderr);

  const calls = fs.readFileSync(s.callLog, 'utf8').trim().split('\n');
  assert.ok(calls.some((l) => /^omarchy-launch-webapp https:\/\/web\.whatsapp\.com\//.test(l)));
  assert.ok(!calls.some((l) => l.startsWith('chromium ')));

  const report = fs.readFileSync(s.outFile, 'utf8');
  assert.match(report, /- WhatsApp Web: your logged-in default profile/);
  assert.match(report, /\| WhatsApp Web \| 300\.0 \| 250\.0 \| 3\.00 \| 5\.0 \|/);
});

test('refuses to start when Chromium is already running', (t) => {
  const s = sandbox(t);
  s.seedFixture('x_chromium', '424242');
  const result = s.run();

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /close these first/);
  assert.match(result.stderr, /Chromium/);
  assert.equal(fs.readFileSync(s.callLog, 'utf8'), '');
  assert.equal(fs.existsSync(s.outFile), false);
});

test('refuses to start when Bambu Studio is already running', (t) => {
  const s = sandbox(t);
  s.seedFixture('x_bambustu_main', '424243');
  const result = s.run();

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Bambu Studio/);
  assert.equal(fs.readFileSync(s.callLog, 'utf8'), '');
});

test('refuses to start when Telegram Desktop is already running', (t) => {
  const s = sandbox(t);
  s.seedFixture('x_Telegram', '424244');
  const result = s.run();

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Telegram Desktop/);
  assert.equal(fs.readFileSync(s.callLog, 'utf8'), '');
});

test('refuses to start when the plugin is not installed', (t) => {
  const s = sandbox(t, { pluginInstalled: false });
  const result = s.run();

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not installed/);
  assert.match(result.stderr, /make install-local/);
  assert.equal(fs.readFileSync(s.callLog, 'utf8'), '');
});

test('refuses to start when Telegram Desktop is not installed', (t) => {
  const s = sandbox(t);
  const result = s.run({ TELEGRAM: 'oma-benchmark-test-no-such-telegram' });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Telegram Desktop/);
  assert.equal(fs.readFileSync(s.callLog, 'utf8'), '');
});

test('refuses to start when Chromium is not installed', (t) => {
  const s = sandbox(t);
  const result = s.run({ CHROMIUM: 'oma-benchmark-test-no-such-chromium' });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Chromium \(oma-benchmark-test-no-such-chromium\) is not installed/);
  assert.equal(fs.readFileSync(s.callLog, 'utf8'), '');
});

test('stops Telegram Desktop when a later step fails, and leaves the shell running', (t) => {
  const s = sandbox(t);
  // Sampler call 3 is the Telegram Desktop sample (1: closed, 2: open, 3: telegram).
  const result = s.run({ FAIL_SAMPLER_CALL: '3' });

  assert.notEqual(result.status, 0);
  const calls = fs.readFileSync(s.callLog, 'utf8').trim().split('\n');
  assert.ok(calls.some((l) => /^omarchy-restart-shell/.test(l)));
  assert.ok(!calls.some((l) => l.startsWith('chromium ')));
  assert.equal(s.alive(s.readPid(path.join(s.root, 'telegram.pid'))), false);
  assert.equal(fs.existsSync(s.outFile), false);
});
