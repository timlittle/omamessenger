// Tests for bin/oma-messenger-service (the launcher) and
// scripts/install-helper.sh, run against a fake release over file://.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const repo = path.resolve(__dirname, '../..');
const version = fs.readFileSync(path.join(repo, 'helper-version'), 'utf8').trim();
const arch = { x64: 'amd64', arm64: 'arm64' }[os.arch()];
const assetName = `oma-messenger-service-linux-${arch}`;

// plugin copies the launcher, installer and pin into a temp plugin dir with
// its own XDG_DATA_HOME, and returns helpers to run them.
function plugin(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'oma-helper-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const dir = path.join(root, 'plugin');
  for (const rel of ['bin/oma-messenger-service', 'scripts/install-helper.sh', 'helper-version']) {
    fs.mkdirSync(path.dirname(path.join(dir, rel)), { recursive: true });
    fs.copyFileSync(path.join(repo, rel), path.join(dir, rel));
    fs.chmodSync(path.join(dir, rel), 0o755);
  }
  const data = path.join(root, 'data');
  const env = (extra) => ({ ...process.env, HOME: root, XDG_DATA_HOME: data, ...extra });
  const run = (rel, args, extra) => spawnSync(path.join(dir, rel), args, { encoding: 'utf8', env: env(extra) });
  return { root, dir, data, run, installed: path.join(data, 'omamessenger', 'bin', `oma-messenger-service-${version}`) };
}

// stub writes an executable that answers --version and echoes its args.
function stub(file, label, reportedVersion = version) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, `#!/bin/sh\nif [ "$1" = --version ]; then echo ${reportedVersion}; exit 0; fi\nprintf "${label}:%s\\n" "$*"\n`);
  fs.chmodSync(file, 0o755);
}

// release builds a fake release directory and returns its file:// base.
function release(root, { reportedVersion = version, sums } = {}) {
  const dir = path.join(root, 'release');
  stub(path.join(dir, assetName), 'release', reportedVersion);
  const digest = crypto.createHash('sha256').update(fs.readFileSync(path.join(dir, assetName))).digest('hex');
  fs.writeFileSync(path.join(dir, 'SHA256SUMS'), sums ?? `${digest}  ${assetName}\n`);
  return { OMA_RELEASE_BASE: `file://${dir}` };
}

test('installs the pinned release, which the launcher then runs', { skip: !arch }, (t) => {
  const p = plugin(t);
  const base = release(p.root);
  assert.equal(p.run('scripts/install-helper.sh', ['--status'], base).status, 1);

  const install = p.run('scripts/install-helper.sh', [], base);
  assert.equal(install.status, 0, install.stderr);
  assert.ok(fs.existsSync(p.installed));
  assert.equal(p.run('scripts/install-helper.sh', ['--status'], base).status, 0);
  assert.match(p.run('scripts/install-helper.sh', [], base).stdout, /already installed/);

  const launched = p.run('bin/oma-messenger-service', ['--data-dir', '/tmp/x']);
  assert.equal(launched.status, 0, launched.stderr);
  assert.equal(launched.stdout, 'release:--data-dir /tmp/x\n');
});

test('refuses a binary whose checksum does not match', { skip: !arch }, (t) => {
  const p = plugin(t);
  const base = release(p.root, { sums: `${'0'.repeat(64)}  ${assetName}\n` });
  const install = p.run('scripts/install-helper.sh', [], base);
  assert.notEqual(install.status, 0);
  assert.match(install.stderr, /checksum mismatch/);
  assert.ok(!fs.existsSync(p.installed));
});

test('refuses a release without a checksum entry', { skip: !arch }, (t) => {
  const p = plugin(t);
  const base = release(p.root, { sums: `${'0'.repeat(64)}  some-other-file\n` });
  const install = p.run('scripts/install-helper.sh', [], base);
  assert.notEqual(install.status, 0);
  assert.match(install.stderr, /no entry/);
  assert.ok(!fs.existsSync(p.installed));
});

test('refuses a binary that reports a different version', { skip: !arch }, (t) => {
  const p = plugin(t);
  const base = release(p.root, { reportedVersion: '9.9.9' });
  const install = p.run('scripts/install-helper.sh', [], base);
  assert.notEqual(install.status, 0);
  assert.match(install.stderr, /reports version 9\.9\.9/);
  assert.ok(!fs.existsSync(p.installed));
});

test('refuses an unreachable release', { skip: !arch }, (t) => {
  const p = plugin(t);
  const install = p.run('scripts/install-helper.sh', [], { OMA_RELEASE_BASE: `file://${p.root}/nowhere` });
  assert.notEqual(install.status, 0);
  assert.match(install.stderr, /could not download/);
});

test('launcher prefers a local development build', (t) => {
  const p = plugin(t);
  stub(p.installed, 'release');
  stub(path.join(p.dir, 'bin', 'dev', 'oma-messenger-service'), 'dev');
  const launched = p.run('bin/oma-messenger-service', ['--data-dir', '/tmp/y']);
  assert.equal(launched.status, 0, launched.stderr);
  assert.equal(launched.stdout, 'dev:--data-dir /tmp/y\n');
});

test('launcher exits 3 with instructions when no helper is installed', (t) => {
  const p = plugin(t);
  const launched = p.run('bin/oma-messenger-service', []);
  assert.equal(launched.status, 3);
  assert.match(launched.stderr, new RegExp(`helper ${version.replace(/\./g, '\\.')} is not installed`));
  assert.match(launched.stderr, /install-helper\.sh/);
});
