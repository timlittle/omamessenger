// Checks on the committed whats-new.json itself, rather than on
// ui/lib/WhatsNew.js's parsing logic (see whatsnew.test.cjs): that its top
// entry names the release just shipped or the one being prepared, and
// that every change line is the plain, short, user-worded sentence the
// overlay expects.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const { load } = require('./load.cjs');

const WhatsNew = load('lib/WhatsNew.js');
const repoRoot = path.join(__dirname, '..', '..');

const manifest = JSON.parse(fs.readFileSync(path.join(repoRoot, 'manifest.json'), 'utf8'));
const releases = WhatsNew.parse(fs.readFileSync(path.join(repoRoot, 'whats-new.json'), 'utf8'));

test('whats-new.json parses into at least one release', () => {
  assert.ok(releases.length > 0, 'expected at least one release in whats-new.json');
});

test("whats-new.json's top entry is the release just shipped or the one being prepared", () => {
  // Between releases the top entry may still be the release just shipped
  // (manifest.json's version) or already describe the next one (one
  // patch ahead, the bump release-please gives pre-1.0). A release pull
  // request bumps manifest.json but never this file, so if nobody added
  // an entry for that release, its top entry is now older than
  // manifest.json and this fails on the release pull request itself,
  // never on main between releases.
  const allowed = [manifest.version, WhatsNew.nextPatchVersion(manifest.version)];
  assert.ok(
    allowed.includes(releases[0].version),
    `whats-new.json's top entry is ${releases[0].version}; add an entry for ${manifest.version} before releasing it`,
  );
});

test('every change line is a short, plain, non-empty sentence', () => {
  for (const release of releases) {
    assert.ok(release.changes.length > 0, `release ${release.version} has no change lines`);
    assert.ok(release.changes.length <= 8, `release ${release.version} has more than 8 change lines`);

    for (const line of release.changes) {
      assert.ok(line.trim().length > 0, `release ${release.version} has a blank change line`);
      assert.ok(line.length <= 120, `release ${release.version} change line is too long: ${line}`);
      assert.ok(!/[<>]/.test(line), `release ${release.version} change line looks like markup: ${line}`);
    }
  }
});
