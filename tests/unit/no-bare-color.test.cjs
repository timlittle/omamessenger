// Regression guard against the bare Omarchy "Color" singleton. Qt 6.12
// added a global QML value type of the same name, which shadows an
// unqualified "Color" identifier once that version is in scope: a plain
// "import qs.Commons" followed by "Color.foreground" then resolves to the
// new global type instead of Omarchy's singleton and throws ("Cannot read
// property 'foreground' of undefined"), breaking the whole UI on Qt 6.12
// while staying silent on the Qt 6.11 this UI was built against (see
// docs/decisions.md and the "qml-next" CI job that tracks this). The fix
// is to never write "Color" unqualified anywhere else: ui/theme/Theme.qml
// is the one file that names Omarchy's Color singleton, through a
// qualified "import qs.Commons as Commons", and every other file reads
// colours from Theme (Theme.foreground, Theme.accent, ...) instead.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// EXEMPT_FILE is the only file allowed to name Omarchy's Color singleton
// directly, since it is the adapter every other file reads colours
// through.
const EXEMPT_FILE = path.join('theme', 'Theme.qml');

// BARE_COLOR_PATTERN matches the identifier "Color" immediately followed
// by "." and not preceded by a word character or a ".", so it catches
// "Color.foreground" but never "AccountColor.colorIndex" or
// "Commons.Color.foreground".
const BARE_COLOR_PATTERN = /(?<![\w.])Color\./g;

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

// findBareColor returns one description per bare "Color." reference in
// src, as "<file>:<line>".
function findBareColor(file, src) {
  const found = [];
  for (const match of src.matchAll(BARE_COLOR_PATTERN)) {
    const line = src.slice(0, match.index).split('\n').length;
    found.push(`${file}:${line}`);
  }
  return found;
}

test('no QML file outside the theme adapter names the bare Color singleton', () => {
  const offenders = qmlFiles(UI_ROOT)
    .map((file) => path.relative(UI_ROOT, file))
    .filter((file) => file !== EXEMPT_FILE)
    .flatMap((file) => findBareColor(file, fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} reference(s) to the bare "Color" singleton, which Qt 6.12's global Color type shadows: ` +
    `read the colour from ui/theme/Theme.qml instead (Theme.foreground, Theme.accent, ...):\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught', () => {
  const offenders = findBareColor('fixture.qml', 'Item {\n  color: Color.foreground\n}\n');

  assert.deepEqual(offenders, ['fixture.qml:2']);
});
