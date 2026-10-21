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
// helper-version carries a trailing "# x-release-please-version" comment
// that release-please's generic updater matches on; drop it before use.
const version = fs.readFileSync(path.join(repo, 'helper-version'), 'utf8').split('#')[0].trim();
const arch = { x64: 'amd64', arm64: 'arm64' }[os.arch()];
const assetName = `oma-messenger-service-linux-${arch}`;
const checksumKey = `oma-messenger-service-${version}-linux-${arch}`;

// plugin copies the launcher, installer and pin into a temp plugin dir with
// its own XDG_DATA_HOME, and returns helpers to run them. The committed
// checksum file lives outside the plugin dir, at OMA_CHECKSUMS_FILE, since
// each test needs its own entry for the fake release it builds rather than
// the real repository's pin.
function plugin(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'oma-helper-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const dir = path.join(root, 'plugin');
  for (const rel of ['bin/oma-messenger-service', 'scripts/install-helper.sh', 'scripts/desktop-entry.sh', 'helper-version']) {
    fs.mkdirSync(path.dirname(path.join(dir, rel)), { recursive: true });
    fs.copyFileSync(path.join(repo, rel), path.join(dir, rel));
    fs.chmodSync(path.join(dir, rel), 0o755);
  }
  const data = path.join(root, 'data');
  const checksumsFile = path.join(root, 'helper-checksums');
  const env = (extra) => ({ ...process.env, HOME: root, XDG_DATA_HOME: data, OMA_CHECKSUMS_FILE: checksumsFile, ...extra });
  const run = (rel, args, extra) => spawnSync(path.join(dir, rel), args, { encoding: 'utf8', env: env(extra) });
  return {
    root, dir, data, checksumsFile, run,
    installed: path.join(data, 'omamessenger', 'bin', `oma-messenger-service-${version}`),
    desktopFile: path.join(data, 'applications', 'io.github.omamessenger.desktop'),
  };
}

// stub writes an executable that answers --version and echoes its args.
function stub(file, label, reportedVersion = version) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, `#!/bin/sh\nif [ "$1" = --version ]; then echo ${reportedVersion}; exit 0; fi\nprintf "${label}:%s\\n" "$*"\n`);
  fs.chmodSync(file, 0o755);
}

// release builds a fake release directory and returns its file:// base and
// the asset's real digest, so a test can commit a matching or mismatched
// checksum for it.
function release(root, { reportedVersion = version, sums } = {}) {
  const dir = path.join(root, 'release');
  stub(path.join(dir, assetName), 'release', reportedVersion);
  const digest = crypto.createHash('sha256').update(fs.readFileSync(path.join(dir, assetName))).digest('hex');
  fs.writeFileSync(path.join(dir, 'SHA256SUMS'), sums ?? `${digest}  ${assetName}\n`);
  return { env: { OMA_RELEASE_BASE: `file://${dir}` }, digest };
}

// pin writes the committed checksum entry a test's install-helper.sh run
// will check the downloaded binary against.
function pin(p, hash) {
  fs.writeFileSync(p.checksumsFile, `${hash}  ${checksumKey}\n`);
}

test('installs the pinned release, which the launcher then runs', { skip: !arch }, (t) => {
  const p = plugin(t);
  const { env, digest } = release(p.root);
  pin(p, digest);
  assert.equal(p.run('scripts/install-helper.sh', ['--status'], env).status, 1);

  const install = p.run('scripts/install-helper.sh', [], env);
  assert.equal(install.status, 0, install.stderr);
  assert.ok(fs.existsSync(p.installed));

  // The data directory and its bin folder are private to this account,
  // not left at the directory's default mode.
  const dataDir = path.join(p.data, 'omamessenger');
  assert.equal(fs.statSync(dataDir).mode & 0o777, 0o700);
  assert.equal(fs.statSync(path.join(dataDir, 'bin')).mode & 0o777, 0o700);

  // Installing the helper also drops OmaMessenger into the apps menu.
  assert.ok(fs.existsSync(p.desktopFile));
  assert.equal(fs.statSync(p.desktopFile).mode & 0o777, 0o644);
  const entry = fs.readFileSync(p.desktopFile, 'utf8');
  assert.match(entry, /^Name=OmaMessenger$/m);
  assert.match(entry, /^Exec=omarchy-shell shell summon io\.github\.omamessenger "\{\}"$/m);

  assert.equal(p.run('scripts/install-helper.sh', ['--status'], env).status, 0);
  assert.match(p.run('scripts/install-helper.sh', [], env).stdout, /already installed/);
  assert.equal(fs.readFileSync(p.desktopFile, 'utf8'), entry);

  const launched = p.run('bin/oma-messenger-service', ['--data-dir', '/tmp/x']);
  assert.equal(launched.status, 0, launched.stderr);
  assert.equal(launched.stdout, 'release:--data-dir /tmp/x\n');
});

test('refuses a binary whose checksum does not match the committed pin', { skip: !arch }, (t) => {
  const p = plugin(t);
  const { env, digest } = release(p.root);
  pin(p, '1'.repeat(64));
  assert.notEqual(digest, '1'.repeat(64));
  const install = p.run('scripts/install-helper.sh', [], env);
  assert.notEqual(install.status, 0);
  assert.match(install.stderr, /checksum mismatch/);
  assert.ok(!fs.existsSync(p.installed));
});

test('refuses a release with no committed checksum entry for the pinned version', { skip: !arch }, (t) => {
  const p = plugin(t);
  const { env, digest } = release(p.root);
  fs.writeFileSync(p.checksumsFile, `${digest}  oma-messenger-service-9.9.9-linux-${arch}\n`);
  const install = p.run('scripts/install-helper.sh', [], env);
  assert.notEqual(install.status, 0);
  assert.match(install.stderr, /no checksum committed/);
  assert.ok(!fs.existsSync(p.installed));
});

test('refuses when a missing committed checksum file has no entry at all', { skip: !arch }, (t) => {
  const p = plugin(t);
  const { env } = release(p.root);
  fs.rmSync(p.checksumsFile, { force: true });
  const install = p.run('scripts/install-helper.sh', [], env);
  assert.notEqual(install.status, 0);
  assert.match(install.stderr, /no committed checksum file/);
  assert.ok(!fs.existsSync(p.installed));
});

test('refuses a tampered SHA256SUMS that matches a tampered binary, against the committed pin', { skip: !arch }, (t) => {
  const p = plugin(t);
  // The genuine release's digest is what the committed pin holds, even
  // though the serving directory never carries that binary: it stands in
  // for "the real release", recorded once and never fetched again.
  const genuine = fs.mkdtempSync(path.join(p.root, 'genuine-'));
  stub(path.join(genuine, assetName), 'release');
  const realDigest = crypto.createHash('sha256').update(fs.readFileSync(path.join(genuine, assetName))).digest('hex');
  pin(p, realDigest);

  // The release's own SHA256SUMS is rewritten to agree with the tampered
  // binary, as an attacker able to replace both would do; the committed
  // pin, from the real release, still catches it.
  const dir = path.join(p.root, 'release');
  stub(path.join(dir, assetName), 'tampered');
  const tamperedDigest = crypto.createHash('sha256').update(fs.readFileSync(path.join(dir, assetName))).digest('hex');
  fs.writeFileSync(path.join(dir, 'SHA256SUMS'), `${tamperedDigest}  ${assetName}\n`);
  assert.notEqual(tamperedDigest, realDigest);
  const env = { OMA_RELEASE_BASE: `file://${dir}` };

  const install = p.run('scripts/install-helper.sh', [], env);
  assert.notEqual(install.status, 0);
  assert.match(install.stderr, /checksum mismatch/);
  assert.ok(!fs.existsSync(p.installed));
});

test('refuses a binary that reports a different version', { skip: !arch }, (t) => {
  const p = plugin(t);
  const { env, digest } = release(p.root, { reportedVersion: '9.9.9' });
  pin(p, digest);
  const install = p.run('scripts/install-helper.sh', [], env);
  assert.notEqual(install.status, 0);
  assert.match(install.stderr, /reports version 9\.9\.9/);
  assert.ok(!fs.existsSync(p.installed));
});

test('refuses an unreachable release', { skip: !arch }, (t) => {
  const p = plugin(t);
  // The committed pin need not match anything real here: the download
  // itself fails before any hash is ever compared.
  pin(p, '0'.repeat(64));
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
