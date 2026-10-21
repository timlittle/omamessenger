// Regression guard against a text input field that exists even while its
// overlay is closed. A palette or account-setup TextField that is merely
// hidden, not destroyed, still exists as a live Qt object: on supported
// Qt versions it keeps reading a foreign Wayland clipboard offer (whoever
// else on the compositor last copied something) with no size or time
// limit, just by being focusable, long before anyone opens that overlay
// or pastes into it. The fix is a Loader whose `active` is bound to the
// overlay's own open state, so the field is created fresh each time the
// overlay opens and destroyed when it closes.
//
// This does not re-check that a Loader's `active` binding reaches the
// right boolean — that is the controller's job, proven in
// tests/qml/Controllers and friends — only that every component holding
// one of these fields is either on the short allow-list of fields that
// are supposed to always be on screen, or is never instantiated directly
// outside a Loader whose `active` is bound to something other than a
// bare `true`.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// ALLOWLIST names every component allowed to hold a text input field
// with no Loader gating it at all, anywhere, each with the one-line
// reason it is safe. Empty: ListColumn's search field and Composer's
// own text input used to be listed here as "always visible, so nothing
// to bound" - true of the component itself, but not of the field inside
// it, which is hidden along with everything else in a narrow window or
// with no conversation open while staying alive underneath. Each now
// gates its own field with an inner Loader instead (see
// fieldSelfGated), so a caller may still instantiate ListColumn or
// Composer freely, uninvolved with this fix.
const ALLOWLIST = {};

// FIELD_PATTERN matches the start of a TextField, TextArea or TextInput
// object declaration, whether bare or namespaced (qs.Ui's "Ui.TextField").
const FIELD_PATTERN = /(?<![\w.])(?:[A-Za-z][\w.]*\.)?(TextField|TextArea|TextInput)\s*\{/g;

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

// blockFor returns the brace-balanced text of the object declaration whose
// "{" starts at openIndex in src, braces included.
function blockFor(src, openIndex) {
  let depth = 0;
  for (let i = openIndex; i < src.length; i++) {
    if (src[i] === '{') depth++;
    else if (src[i] === '}' && --depth === 0) return { text: src.slice(openIndex, i + 1), end: i + 1 };
  }
  throw new Error('unbalanced braces');
}

// hasField reports whether src declares a TextField, TextArea or
// TextInput anywhere in it. FIELD_PATTERN is global and therefore
// stateful between calls, so lastIndex is reset first.
function hasField(src) {
  FIELD_PATTERN.lastIndex = 0;
  return FIELD_PATTERN.test(src);
}

// loaderBlocks returns every "Loader { ... }" block in src, as
// { start, end, text } brace-balanced spans.
function loaderBlocks(src) {
  const blocks = [];
  for (const match of src.matchAll(/Loader\s*\{/g)) {
    const openIndex = match.index + match[0].length - 1;
    const { text, end } = blockFor(src, openIndex);
    blocks.push({ start: match.index, end, text });
  }
  return blocks;
}

// isGated reports whether the component usage at index in src sits inside
// a Loader block that both loads a sourceComponent and binds active to
// something other than a bare "true" (a Loader with active: true is
// always active, which defeats the point of gating).
function isGated(src, index) {
  return loaderBlocks(src).some((block) =>
    index >= block.start && index < block.end &&
    /\bsourceComponent\s*:/.test(block.text) &&
    /\bactive\s*:/.test(block.text) &&
    !/\bactive\s*:\s*true\b/.test(block.text));
}

// findUngatedUsages returns one description per occurrence of
// componentName used as an object type anywhere in src that is not
// inside a gated Loader block, as "<file>:<line>".
function findUngatedUsages(file, src, componentName) {
  const pattern = new RegExp(`(?<![\\w.])${componentName}\\s*\\{`, 'g');
  const found = [];
  for (const match of src.matchAll(pattern)) {
    if (isGated(src, match.index)) continue;
    const line = src.slice(0, match.index).split('\n').length;
    found.push(`${file}:${line}`);
  }
  return found;
}

// fieldSelfGated reports whether every TextField, TextArea and
// TextInput declaration in src sits inside a gated Loader block within
// that same file: a component such as ListColumn or Composer that must
// always exist itself, but whose one field inside it must not, gates
// that field with its own inner Loader rather than asking every caller
// to wrap the whole component in one (see each field's own comment).
// A component this is true for needs no entry in ALLOWLIST and no
// caller-side check at all: nothing about how it is instantiated
// matters to this fix, since the field's own lifetime no longer
// follows the component's.
function fieldSelfGated(src) {
  FIELD_PATTERN.lastIndex = 0;
  for (const match of src.matchAll(FIELD_PATTERN)) {
    if (!isGated(src, match.index)) return false;
  }
  return true;
}

test('every TextField, TextArea and TextInput outside the allow-list is only ever instantiated behind a gated Loader', () => {
  const files = qmlFiles(UI_ROOT).map((file) => path.relative(UI_ROOT, file));
  const fieldFiles = files.filter((file) => hasField(fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));
  const toCheck = fieldFiles.filter((file) =>
    !(file in ALLOWLIST) && !fieldSelfGated(fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  const offenders = [];
  for (const fieldFile of toCheck) {
    const componentName = path.basename(fieldFile, '.qml');
    for (const file of files) {
      if (file === fieldFile) continue; // a file's own definition never names itself as an object type
      const src = fs.readFileSync(path.join(UI_ROOT, file), 'utf8');
      offenders.push(...findUngatedUsages(file, src, componentName));
    }
  }

  assert.deepEqual(offenders, [],
    `${offenders.length} usage(s) of a text-input-holding component outside a Loader gated on its own open ` +
    `state: a field that exists while its overlay is closed can read a foreign Wayland clipboard offer with no ` +
    `size or time limit. Either add the component to this test's ALLOWLIST with a one-line reason it is always ` +
    `on screen, or load it through a Loader whose active binds to the overlay's own open state:\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught', () => {
  const offenders = findUngatedUsages('fixture.qml', 'Item {\n  Dummy {\n    open: true\n  }\n}\n', 'Dummy');

  assert.deepEqual(offenders, ['fixture.qml:2']);
});

test('a Loader with active bound to an overlay state gates its usage', () => {
  const src = 'Item {\n  Loader {\n    active: controller.open\n    sourceComponent: Dummy {\n      open: true\n    }\n  }\n}\n';

  assert.deepEqual(findUngatedUsages('fixture.qml', src, 'Dummy'), []);
});

test('a Loader with active hard-coded to true does not gate its usage', () => {
  const src = 'Item {\n  Loader {\n    active: true\n    sourceComponent: Dummy {\n      open: true\n    }\n  }\n}\n';

  assert.deepEqual(findUngatedUsages('fixture.qml', src, 'Dummy'), ['fixture.qml:4']);
});

test('fieldSelfGated is true when every field in the file sits behind its own gated Loader', () => {
  const src = 'Item {\n  Loader {\n    active: root.shown\n    sourceComponent: Component {\n      TextField {\n        id: f\n      }\n    }\n  }\n}\n';

  assert.equal(fieldSelfGated(src), true);
});

test('fieldSelfGated is false when any field in the file is not gated at all', () => {
  const src = 'Item {\n  Loader {\n    active: root.shown\n    sourceComponent: Component {\n      TextField {\n        id: f\n      }\n    }\n  }\n  TextArea {\n    id: g\n  }\n}\n';

  assert.equal(fieldSelfGated(src), false);
});

test("ListColumn's search field and Composer's own input are both self-gated, needing no caller-side Loader", () => {
  for (const file of ['components/ListColumn.qml', 'components/Composer.qml']) {
    const src = fs.readFileSync(path.join(UI_ROOT, file), 'utf8');
    assert.equal(fieldSelfGated(src), true, `${file} has a field not gated by its own inner Loader`);
  }
});
