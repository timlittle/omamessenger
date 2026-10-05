const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

test('launcher prefers an executable local development helper', (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'oma-launcher-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const binDir = path.join(dir, 'bin');
  const devDir = path.join(binDir, 'dev');
  fs.mkdirSync(devDir, { recursive: true });
  const launcher = path.join(binDir, 'oma-messenger-service');
  fs.copyFileSync(path.resolve(__dirname, '../../bin/oma-messenger-service'), launcher);
  fs.chmodSync(launcher, 0o755);
  const helper = path.join(devDir, 'oma-messenger-service');
  fs.writeFileSync(helper, '#!/bin/sh\nprintf "dev:%s\\n" "$*"\n');
  fs.chmodSync(helper, 0o755);

  const result = spawnSync(launcher, ['--demo', '--seed', '12'], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, 'dev:--demo --seed 12\n');
});
