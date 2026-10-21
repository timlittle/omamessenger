// Tests for ui/lib/WhatsNew.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const WhatsNew = load('lib/WhatsNew.js');

test('parse reads a well-formed file into its releases, newest first', () => {
  const json = JSON.stringify({
    releases: [
      { version: '0.3.5', date: '2026-10-21', changes: ['First change', 'Second change'] },
      { version: '0.3.4', date: '2026-09-01', changes: ['Older change'] }
    ]
  });

  assert.deepEqual(WhatsNew.parse(json), [
    { version: '0.3.5', date: '2026-10-21', changes: ['First change', 'Second change'] },
    { version: '0.3.4', date: '2026-09-01', changes: ['Older change'] }
  ]);
});

test('parse returns an empty list for malformed JSON rather than throwing', () => {
  assert.deepEqual(WhatsNew.parse('{not json'), []);
});

test('parse returns an empty list when "releases" is missing or not an array', () => {
  assert.deepEqual(WhatsNew.parse(JSON.stringify({})), []);
  assert.deepEqual(WhatsNew.parse(JSON.stringify({ releases: 'nope' })), []);
});

test('parse drops a release with no version string', () => {
  const json = JSON.stringify({ releases: [{ date: '2026-10-21', changes: ['x'] }] });
  assert.deepEqual(WhatsNew.parse(json), []);
});

test('parse filters out non-string or blank change lines, keeping the rest', () => {
  const json = JSON.stringify({
    releases: [{ version: '0.3.5', date: '2026-10-21', changes: ['Real change', '', '   ', 42, null] }]
  });

  assert.deepEqual(WhatsNew.parse(json), [{ version: '0.3.5', date: '2026-10-21', changes: ['Real change'] }]);
});

test('parse defaults a missing date or changes list rather than failing', () => {
  const json = JSON.stringify({ releases: [{ version: '0.3.5' }] });
  assert.deepEqual(WhatsNew.parse(json), [{ version: '0.3.5', date: '', changes: [] }]);
});

const shouldShowHintCases = [
  ['a fresh install (no seen version) never shows the hint', '0.3.5', '', false],
  ['an older seen version shows the hint', '0.3.5', '0.3.4', true],
  ['the same seen version does not show the hint again', '0.3.5', '0.3.5', false],
  ['a newer seen version (a downgrade) does not show the hint', '0.3.4', '0.3.5', false],
  ['no current version never shows the hint', '', '0.3.4', false]
];

for (const [name, current, seen, want] of shouldShowHintCases) {
  test(`shouldShowHint: ${name}`, () => {
    assert.equal(WhatsNew.shouldShowHint(current, seen), want);
  });
}

test('hintText names the version and points at the command palette', () => {
  assert.equal(WhatsNew.hintText('0.3.5'), "Updated to v0.3.5 · What's new is in the command palette");
});

const nextPatchVersionCases = [
  ['bumps a plain three-part version', '0.3.4', '0.3.5'],
  ['bumps a version already at a double-digit patch', '0.3.9', '0.3.10'],
  ['fills in a missing patch component as 0 first', '0.3', '0.3.1']
];

for (const [name, version, want] of nextPatchVersionCases) {
  test(`nextPatchVersion: ${name}`, () => {
    assert.equal(WhatsNew.nextPatchVersion(version), want);
  });
}
