// Regression guard against a sender- or service-controlled URL opening
// with any scheme. Qt.openUrlExternally opens whatever string it is
// given with no filter of its own: a WhatsApp link preview's matched
// text, a Telegram formatting span, or a service's own setup error text
// is not something this UI should ever hand it unchecked. Every call
// that might carry such a URL goes through WindowController's own
// openExternalLink, the one place that checks ui/lib/Links.js's
// isAllowed first (see docs/decisions.md); a handful of other files
// call Qt.openUrlExternally directly, but only ever with a local
// "file://" path this helper or the user already put on disk
// themselves (a downloaded photo or video, this plugin's own
// keys.conf), never sender- or service-controlled text, so Links'
// scheme check does not apply to them.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// OPEN_URL_PATTERN matches a direct call to Qt.openUrlExternally.
const OPEN_URL_PATTERN = /Qt\.openUrlExternally\(/g;

// ALLOWED_FILES are the only files that may call Qt.openUrlExternally
// directly, relative to ui/.
const ALLOWED_FILES = new Set([
  'controllers/WindowController.qml',
  'Service.qml',
  'controllers/ConversationController.qml',
  'controllers/MediaController.qml',
  'controllers/PhotoViewerController.qml'
]);

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
// call in src, as "<file>:<line>", when file is not in ALLOWED_FILES.
function findDisallowedOpens(file, src) {
  if (ALLOWED_FILES.has(file)) {
    return [];
  }

  const found = [];
  for (const match of src.matchAll(OPEN_URL_PATTERN)) {
    const line = src.slice(0, match.index).split('\n').length;
    found.push(`${file}:${line}`);
  }
  return found;
}

test('no QML file outside the allowed few calls Qt.openUrlExternally directly', () => {
  const offenders = qmlFiles(UI_ROOT)
    .map((file) => path.relative(UI_ROOT, file))
    .flatMap((file) => findDisallowedOpens(file, fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} direct Qt.openUrlExternally call(s) outside the allowed files: a URL from a message, ` +
    `a link preview or a service's own text must go through WindowController.openExternalLink, which checks ` +
    `ui/lib/Links.js's isAllowed first:\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught', () => {
  const offenders = findDisallowedOpens('components/Example.qml', 'onClicked: Qt.openUrlExternally(url)');

  assert.deepEqual(offenders, ['components/Example.qml:1']);
});

test('the allowed files are not flagged for their own local file:// opens', () => {
  for (const file of ALLOWED_FILES) {
    const src = fs.readFileSync(path.join(UI_ROOT, file), 'utf8');
    assert.deepEqual(findDisallowedOpens(file, src), []);
  }
});
