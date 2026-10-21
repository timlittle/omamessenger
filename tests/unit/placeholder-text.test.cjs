// Regression guard for a field's hint text. A TextField or TextArea's
// placeholderText is drawn by the style's own PlaceholderText item, which
// sets no textFormat and so sniffs its string as AutoText: anything that
// looks like HTML, such as a chat title carrying <img src=...>, renders as
// rich text and fetches a remote image, however the field's own
// textFormat is set. So every placeholderText in ui/ must be built from
// string literals alone, or come from a property listed below whose
// values are themselves only fixed strings written in this repository.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// ALLOWED_SOURCES are non-literal placeholder expressions whose every
// possible value is a fixed string from this repository, each with why.
const ALLOWED_SOURCES = {
  'components/CommandPalette.qml': {
    'root.placeholder': 'set only by Overlays.qml from a choice of fixed strings',
  },
  'components/AccountSetup.qml': {
    'root._field ? root._field.placeholder : ""': 'ui/lib/Setup.js FIELDS, fixed strings',
  },
};

// LITERAL_ONLY matches an expression made only of string literals joined by
// a ternary on any condition, such as `a ? "Blocked" : "Message"`.
const LITERAL_ONLY = /^(?:[^?:"']+\?\s*)?(?:"[^"]*"|'[^']*')(?:\s*:\s*(?:"[^"]*"|'[^']*'))?$/;

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

// placeholderViolations returns each placeholderText in source, a file at
// rel under ui/, whose expression is neither literal-only nor allowed.
function placeholderViolations(rel, source) {
  const allowed = ALLOWED_SOURCES[rel] || {};
  const out = [];
  for (const match of source.matchAll(/^\s*placeholderText:\s*(.+?)\s*$/gm)) {
    const expr = match[1];
    if (LITERAL_ONLY.test(expr) || expr in allowed) continue;
    out.push(`${rel}: placeholderText: ${expr}`);
  }
  return out;
}

test('every placeholderText in ui/ is a fixed string', () => {
  const violations = [];
  for (const file of qmlFiles(UI_ROOT)) {
    const rel = path.relative(UI_ROOT, file);
    violations.push(...placeholderViolations(rel, fs.readFileSync(file, 'utf8')));
  }
  assert.deepEqual(violations, []);
});

test('a placeholder built from a chat title is caught', () => {
  const source = 'TextArea {\n  placeholderText: "Message " + root.title\n}\n';
  assert.deepEqual(placeholderViolations('components/Fake.qml', source), [
    'components/Fake.qml: placeholderText: "Message " + root.title',
  ]);
});

test('literal and ternary-of-literal placeholders pass', () => {
  const source = 'placeholderText: "Search"\nplaceholderText: root.blocked ? "Blocked" : "Message"\n';
  assert.deepEqual(placeholderViolations('components/Fake.qml', source), []);
});
