// Tests for scripts/benchmark.sh, the read-only /proc sampler. It runs
// against short-lived sleep processes this test starts itself, never
// anything else on the machine.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const script = path.resolve(__dirname, '../../scripts/benchmark.sh');

// sampleEnv is the quickest timing the sampler accepts: no settle, one
// sample a second apart.
const sampleEnv = { ...process.env, BENCH_SETTLE: '0', BENCH_SAMPLES: '1', BENCH_INTERVAL: '1' };

// startSleeper starts a detached process that lives for seconds, killed
// when the test ends. It is double-forked so it is reaped as soon as it
// exits rather than lingering as this test's zombie, which /proc would
// still list while the sampler runs.
function startSleeper(t, seconds) {
  const out = spawnSync('sh', ['-c', `sleep ${seconds} >/dev/null 2>&1 & echo $!`], { encoding: 'utf8' });
  const pid = Number(out.stdout.trim());
  t.after(() => {
    try {
      process.kill(pid);
    } catch {
      // Already gone, which is the point for the brief one.
    }
  });
  return pid;
}

test('keeps sampling when one of a label\'s processes exits', (t) => {
  const lasting = startSleeper(t, 30);
  const brief = startSleeper(t, 0.5);

  // The brief one exits during the settle time, before the first reading.
  const env = { ...sampleEnv, BENCH_SETTLE: '1' };
  const result = spawnSync(script, [`browser=${lasting},${brief}`], { env, encoding: 'utf8' });

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /^\| browser \| [\d.]+ \|/m);
});

test('fails when none of a label\'s processes is running', () => {
  const result = spawnSync(script, ['gone=999999999'], { env: sampleEnv, encoding: 'utf8' });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /no process of gone is running/);
});
