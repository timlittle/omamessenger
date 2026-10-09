// Tests for scripts/check-leaks.sh. It runs against a temporary checkout
// and temp directory, with docker stubbed on PATH, so it never inspects
// or changes the real machine.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const script = path.resolve(__dirname, '../../scripts/check-leaks.sh');

// sandbox makes a fake checkout, temp directory and docker stub that
// lists containers, and returns a run function over them.
function sandbox(t, containers = []) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'leaks-test-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const bin = path.join(dir, 'bin');
  const tmp = path.join(dir, 'tmp');
  const checkout = path.join(dir, 'checkout');
  for (const d of [bin, tmp, checkout]) fs.mkdirSync(d);
  fs.writeFileSync(path.join(bin, 'docker'), `#!/bin/sh\nprintf '%s\\n' ${containers.map((c) => `'${c}'`).join(' ')}\n`);
  fs.chmodSync(path.join(bin, 'docker'), 0o755);

  const run = () => spawnSync('sh', [script], {
    encoding: 'utf8',
    env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, LEAK_TMP: tmp, LEAK_ROOT: checkout },
  });
  return { tmp, run };
}

test('passes when nothing is left behind', (t) => {
  const s = sandbox(t);
  const result = s.run();

  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stderr, '');
});

test('fails on a leftover act container', (t) => {
  const s = sandbox(t, ['act-CI-check-123']);
  const result = s.run();

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /container act-CI-check-123 is still there/);
});

test('fails on scratch left in the temp directory', (t) => {
  const s = sandbox(t);
  fs.mkdirSync(path.join(s.tmp, 'oma-go-cache'));
  const result = s.run();

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /oma-go-cache is in a RAM-backed temp directory/);
});
