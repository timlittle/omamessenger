// Two regression guards for how this UI opens a local file or a URL
// from outside it.
//
// The first is against any QML file ever starting a process - Qt's
// Qt.openUrlExternally runs xdg-open (or the detected browser)
// directly - whose own command-line arguments would carry a
// sender-, service- or user-controlled target: another local user can
// read a process's full argv through /proc/<pid>/cmdline for as long
// as it lives, and a link can carry something as sensitive as a
// password-reset token (see docs/decisions.md). Every open, local
// file or remote link alike, now asks the helper's own links.open or
// files.open method instead, which opens it through the desktop
// portal over D-Bus - a method call's own arguments are never a
// process's argv - rather than falling back to Qt.openUrlExternally
// when the portal cannot be reached. WindowController's own
// openExternalLink additionally checks ui/lib/Links.js's isAllowed
// before ever asking the helper to open a link.
//
// The second is against building a local "file://" URL by hand, for
// showing it in an Image or MediaPlayer source rather than opening it
// externally (opening now never builds one of these URLs at all,
// having moved to files.open's own plain path parameter instead). A
// local path, however it was named, can still hold a character -
// '#', '?', '%', a space, non-ASCII text - that a raw `"file://" +
// path` concatenation would mangle into the wrong URL rather than
// merely refuse (see ui/lib/Links.js's fileURL and docs/decisions.md).
// Every such local URL goes through fileURL instead.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// OPEN_URL_PATTERN matches a direct call to Qt.openUrlExternally.
const OPEN_URL_PATTERN = /Qt\.openUrlExternally\(/g;

// qmlFiles walks dir and returns every ".qml" file under it.
function qmlFiles(dir) {
  const found = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      found.push(...qmlFiles(full));
    } else if (entry.name.endsWith('.qml')) {
      found.push(full);
    }
  }
  return found;
}

// findDisallowedOpens returns one description per Qt.openUrlExternally
// call in src, as "<file>:<line>": every one is disallowed now, since
// opening anything, local or remote, goes through the helper's own
// links.open or files.open method instead (see this file's own header
// comment).
function findDisallowedOpens(file, src) {
  const found = [];
  for (const match of src.matchAll(OPEN_URL_PATTERN)) {
    const line = src.slice(0, match.index).split('\n').length;
    found.push(`${file}:${line}`);
  }
  return found;
}

test('no QML file calls Qt.openUrlExternally directly', () => {
  const offenders = qmlFiles(UI_ROOT)
    .map((file) => path.relative(UI_ROOT, file))
    .flatMap((file) => findDisallowedOpens(file, fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} direct Qt.openUrlExternally call(s): a link or a local file must be opened through ` +
    `the helper's own links.open or files.open method instead, over D-Bus through the desktop portal, never ` +
    `by starting a process whose argv would carry the target (see docs/decisions.md):\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught', () => {
  const offenders = findDisallowedOpens('components/Example.qml', 'onClicked: Qt.openUrlExternally(url)');

  assert.deepEqual(offenders, ['components/Example.qml:1']);
});

// RAW_FILE_URL_PATTERN matches a "file://" URL built by hand - string
// concatenation - rather than through Links.fileURL's percent-encoding.
const RAW_FILE_URL_PATTERN = /["']file:\/\/["']\s*\+/g;

// findRawFileURLs returns one description per hand-built "file://" URL
// in src, as "<file>:<line>".
function findRawFileURLs(file, src) {
  const found = [];
  for (const match of src.matchAll(RAW_FILE_URL_PATTERN)) {
    const line = src.slice(0, match.index).split('\n').length;
    found.push(`${file}:${line}`);
  }
  return found;
}

test('no QML file builds a "file://" URL by hand instead of through Links.fileURL', () => {
  const offenders = qmlFiles(UI_ROOT)
    .map((file) => path.relative(UI_ROOT, file))
    .flatMap((file) => findRawFileURLs(file, fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} hand-built "file://" URL(s), unpercent-encoded: every local file this UI opens or ` +
    `shows must go through ui/lib/Links.js's fileURL instead:\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught for the raw file:// check too', () => {
  const offenders = findRawFileURLs('components/Example.qml', 'source: "file://" + root.path');

  assert.deepEqual(offenders, ['components/Example.qml:1']);
});
