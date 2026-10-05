const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

// The bundled binary name for this machine, as the launcher derives it.
const archTarget = { x64: 'linux-amd64', arm64: 'linux-arm64' }[os.arch()];

// launcherIn copies the launcher into a fresh bin directory.
function launcherIn(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'oma-launcher-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const binDir = path.join(dir, 'bin');
  fs.mkdirSync(path.join(binDir, 'dev'), { recursive: true });
  const launcher = path.join(binDir, 'oma-messenger-service');
  fs.copyFileSync(path.resolve(__dirname, '../../bin/oma-messenger-service'), launcher);
  fs.chmodSync(launcher, 0o755);
  return { binDir, launcher };
}

function stub(file, label) {
  fs.writeFileSync(file, `#!/bin/sh\nprintf "${label}:%s\\n" "$*"\n`);
  fs.chmodSync(file, 0o755);
}

test('launcher prefers an executable local development helper', (t) => {
  const { binDir, launcher } = launcherIn(t);
  stub(path.join(binDir, 'dev', 'oma-messenger-service'), 'dev');
  if (archTarget) stub(path.join(binDir, `oma-messenger-service-${archTarget}`), 'release');

  const result = spawnSync(launcher, ['--demo', '--seed', '12'], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, 'dev:--demo --seed 12\n');
});

test('launcher falls back to the bundled helper for this architecture', { skip: !archTarget }, (t) => {
  const { binDir, launcher } = launcherIn(t);
  stub(path.join(binDir, `oma-messenger-service-${archTarget}`), 'release');

  const result = spawnSync(launcher, ['--demo'], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, 'release:--demo\n');
});

test('launcher fails with a message when no helper is present', (t) => {
  const { launcher } = launcherIn(t);

  const result = spawnSync(launcher, [], { encoding: 'utf8' });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /OmaMessenger (helper is missing|does not include a helper)/);
});
