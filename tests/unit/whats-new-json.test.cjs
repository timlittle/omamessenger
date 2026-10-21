// Checks on the committed whats-new.json itself, rather than on
// ui/lib/WhatsNew.js's parsing logic (see whatsnew.test.cjs): that it is
// seeded for the release release-please will cut next, and that every
// change line is the plain, short, user-worded sentence the overlay
// expects.
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

test("whats-new.json's top entry is seeded for the release release-please will cut next", () => {
  // release-please bumps the patch component pre-1.0 for a "feat" or
  // "fix" commit (bump-patch-for-minor-pre-major in
  // release-please-config.json), so the version it will give the next
  // release is manifest.json's current version plus one patch - not
  // manifest.json's version itself, which still names the *last*
  // release. Comparing against that predicted version, rather than
  // manifest.json's directly, is what lets this pass right now, between
  // releases, while the top entry describes the still-unreleased
  // version. release-please's own extra-files sync (see
  // release-please-config.json) rewrites this same field to the exact
  // version it computed in the release pull request, and once that
  // lands this check fails again until a new top entry is added for the
  // version after - which is the point: a release with no curated
  // "What's new" entry prepared for it fails `make check`.
  assert.equal(releases[0].version, WhatsNew.nextPatchVersion(manifest.version));
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
