// Tests for scripts/install-local.sh: run against stub `omarchy` and
// `rsync` binaries and a temporary HOME/XDG_DATA_HOME, never the real
// desktop session.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const repo = path.resolve(__dirname, '../..');
const desktopValidate = spawnSync('desktop-file-validate', ['--version']).error == null;

// stub writes an executable that does nothing but exit 0, standing in
// for the real `omarchy` and `rsync` so the test never touches either.
function stub(dir, name) {
  const file = path.join(dir, name);
  fs.writeFileSync(file, '#!/bin/sh\nexit 0\n');
  fs.chmodSync(file, 0o755);
}

// sandbox builds a temporary HOME, XDG_DATA_HOME and a PATH that resolves
// `omarchy` and `rsync` to stubs ahead of any real ones, and returns the
// env to run install-local.sh with plus where the desktop entry lands.
function sandbox(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'oma-install-local-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const home = path.join(root, 'home');
  const data = path.join(root, 'data');
  const stubBin = path.join(root, 'stub-bin');
  fs.mkdirSync(home, { recursive: true });
  fs.mkdirSync(data, { recursive: true });
  fs.mkdirSync(stubBin, { recursive: true });
  stub(stubBin, 'omarchy');
  stub(stubBin, 'rsync');
  const env = {
    ...process.env,
    HOME: home,
    XDG_DATA_HOME: data,
    PATH: `${stubBin}:${process.env.PATH}`,
  };
  return { env, desktopFile: path.join(data, 'applications', 'io.github.omamessenger.desktop') };
}

test('installing the plugin adds an apps-menu entry', (t) => {
  const { env, desktopFile } = sandbox(t);
  const install = spawnSync(path.join(repo, 'scripts/install-local.sh'), [], { encoding: 'utf8', env });
  assert.equal(install.status, 0, install.stderr);

  assert.ok(fs.existsSync(desktopFile));
  assert.equal(fs.statSync(desktopFile).mode & 0o777, 0o644);
  const content = fs.readFileSync(desktopFile, 'utf8');
  assert.match(content, /^Type=Application$/m);
  assert.match(content, /^Name=OmaMessenger$/m);
  assert.match(content, /^Exec=omarchy-shell shell summon io\.github\.omamessenger "\{\}"$/m);

  if (desktopValidate) {
    const validate = spawnSync('desktop-file-validate', [desktopFile], { encoding: 'utf8' });
    assert.equal(validate.status, 0, validate.stdout + validate.stderr);
  }
});

test('installing the plugin twice leaves one, unchanged entry', (t) => {
  const { env, desktopFile } = sandbox(t);
  const run = () => spawnSync(path.join(repo, 'scripts/install-local.sh'), [], { encoding: 'utf8', env });

  assert.equal(run().status, 0);
  const first = fs.readFileSync(desktopFile, 'utf8');
  assert.equal(run().status, 0);
  const second = fs.readFileSync(desktopFile, 'utf8');
  assert.equal(first, second);
});
