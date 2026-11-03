// Regression guard against a Qt text field reading the clipboard
// outside the helper's own bounded clipboard.readText. A follow-up
// security review found every editable field but the composer's still
// let Qt's own TextField/TextArea handle Ctrl+V, Shift+Insert,
// middle-click (the Wayland primary selection) and its own default
// right-click "Paste" menu directly - each an in-process read with no
// size or time limit of its own, unlike the bounded helper round trip
// (see docs/decisions.md and ClipboardPaste.qml).
//
// This checks every TextField, TextArea and TextInput declared
// anywhere in ui/ carries the three markers that close those paths:
//   - `maximumLength:`, so even a paste or a burst of typing is capped
//     at the field's own declared limit (ui/lib/Limits.js), not only
//     at whatever the helper's own clipboard.readText already caps at
//   - `ContextMenu.menu: null`, removing Qt's own right-click "Paste"
//     outright
//   - a child `MiddleClickGuard {}`, swallowing the primary-selection
//     middle-click paste
// A single-line field also falls through to `root.routeKey` in its own
// `Keys.onPressed`, the one thing that lets Ctrl+V and Shift+Insert
// reach Panel.qml's own "field.paste" dispatch (see Keymap.js and
// Panel.qml) rather than Qt's own built-in paste; this is checked too.
//
// The composer's own TextArea is the one documented exception
// (COMPOSER_FIELD below): it has its own larger paste path
// (ComposerController.paste, which tries a pasted image first) and its
// own Unicode-aware length cap (SpellCheck.capComposerText) since
// TextArea has no maximumLength property to declare at all - both
// already covered by tests/qml/ComposerController and
// tests/unit/spellcheck.test.cjs, not re-proven here.
//
// A follow-up review found Ctrl+V and Shift+Insert are not the only
// keys that reach a field's own built-in paste(): Qt's
// QKeySequence::StandardKey::Paste also covers Ctrl+Shift+Insert and,
// on some platform themes, the dedicated Paste key, and the binding
// lookup in Keymap.js/Panel.qml only ever matches the two keys listed
// there - worse, keys.conf can rebind field.paste or compose.attach
// off Ctrl+V entirely, so even that key would then fall through too.
// Every field - the composer included this time - must therefore
// catch a native paste key itself, ahead of the routeKey binding
// lookup, and force the same bounded path regardless of bindings
// (Panel.qml's routeKey takes a `nativePaste` flag for exactly this;
// see docs/decisions.md).
//
// A later review found event.matches(StandardKey.Paste) itself unsafe
// to depend on: Qt resolves it from the desktop's active platform
// theme, which a developer's own session sets and a bare CI container
// never does, so the same key event matches there and not here. Every
// field now calls ui/lib/Paste.js's own isPasteKey(event.key,
// event.modifiers) instead - a fixed list of every key some keyboard
// scheme maps to Paste, plus the dedicated Paste key, that answers the
// same way regardless of the platform theme (see docs/decisions.md).
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// COMPOSER_FIELD is the one field this test does not require
// maximumLength or a MiddleClickGuard from; see the file comment above.
const COMPOSER_FIELD = { file: 'components/Composer.qml', objectName: 'composerInput' };

// FIELD_PATTERN matches the start of a TextField, TextArea or TextInput
// object declaration, whether bare or namespaced ("qs.Ui's Ui.TextField").
const FIELD_PATTERN = /(?<![\w.])(?:[A-Za-z][\w.]*\.)?(TextField|TextArea|TextInput)\s*\{/g;

// qmlFiles walks dir and returns every ".qml" file under it.
function qmlFiles(dir) {
  const found = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) found.push(...qmlFiles(full));
    else if (entry.name.endsWith('.qml')) found.push(full);
  }
  return found;
}

// blockFor returns the brace-balanced text of the object declaration whose
// "{" starts at openIndex in src, braces included.
function blockFor(src, openIndex) {
  let depth = 0;
  for (let i = openIndex; i < src.length; i++) {
    if (src[i] === '{') depth++;
    else if (src[i] === '}' && --depth === 0) return src.slice(openIndex, i + 1);
  }
  throw new Error('unbalanced braces');
}

// objectNameOf returns block's own objectName value, or "" when it has
// none (every field this test cares about declares one).
function objectNameOf(block) {
  const match = block.match(/objectName\s*:\s*"([^"]*)"/);
  return match ? match[1] : '';
}

// fields returns every { file, objectName, isTextArea, block } found
// under UI_ROOT: one entry per TextField/TextArea/TextInput declaration.
function fields() {
  const found = [];
  for (const file of qmlFiles(UI_ROOT)) {
    const relative = path.relative(UI_ROOT, file);
    const src = fs.readFileSync(file, 'utf8');

    for (const match of src.matchAll(FIELD_PATTERN)) {
      const openIndex = match.index + match[0].length - 1;
      const block = blockFor(src, openIndex);
      found.push({ file: relative, objectName: objectNameOf(block), isTextArea: match[1] === 'TextArea', block });
    }
  }
  return found;
}

// isComposerField reports whether field is the one documented exception.
function isComposerField(field) {
  return field.file === COMPOSER_FIELD.file && field.objectName === COMPOSER_FIELD.objectName;
}

test('every editable field but the composer declares maximumLength', () => {
  const offenders = fields()
    .filter((f) => !isComposerField(f) && !/\bmaximumLength\s*:/.test(f.block))
    .map((f) => `${f.file} (${f.objectName || 'unnamed'})`);

  assert.deepEqual(offenders, [],
    `${offenders.length} field(s) with no maximumLength, so even a paste or a burst of typing is unbounded ` +
    `(see ui/lib/Limits.js):\n  ${offenders.join('\n  ')}`);
});

test('every editable field removes Qt\'s own right-click paste menu', () => {
  const offenders = fields()
    .filter((f) => !/\bContextMenu\.menu\s*:\s*null\b/.test(f.block))
    .map((f) => `${f.file} (${f.objectName || 'unnamed'})`);

  assert.deepEqual(offenders, [],
    `${offenders.length} field(s) with no "ContextMenu.menu: null", so Qt's own right-click Paste can still read ` +
    `the clipboard in-process, with no size or time limit of its own:\n  ${offenders.join('\n  ')}`);
});

test('every editable field but the composer swallows a middle-click with MiddleClickGuard', () => {
  // The composer's own TextArea swallows middle-click a different way
  // (the spellRightClick MouseArea next to it, which also opens the
  // spelling picker on a right-click), already proven in
  // tests/qml/Composer; this only checks every other field uses the
  // shared component.
  const offenders = fields()
    .filter((f) => !isComposerField(f) && !/MiddleClickGuard\s*\{/.test(f.block))
    .map((f) => `${f.file} (${f.objectName || 'unnamed'})`);

  assert.deepEqual(offenders, [],
    `${offenders.length} field(s) with no MiddleClickGuard, so a middle-click still pastes the Wayland primary ` +
    `selection straight through Qt's own unbounded reader:\n  ${offenders.join('\n  ')}`);
});

test('every single-line field falls through to routeKey, reaching Panel.qml\'s field.paste dispatch', () => {
  const offenders = fields()
    .filter((f) => !f.isTextArea && !/root\.routeKey\s*&&\s*root\.routeKey\(/.test(f.block))
    .map((f) => `${f.file} (${f.objectName || 'unnamed'})`);

  assert.deepEqual(offenders, [],
    `${offenders.length} field(s) do not forward an unhandled key press to root.routeKey, so Ctrl+V and ` +
    `Shift+Insert never reach the helper's bounded clipboard.readText there:\n  ${offenders.join('\n  ')}`);
});

test('every field, including the composer, catches Paste.isPasteKey before routeKey\'s binding lookup', () => {
  const offenders = fields()
    .filter((f) => {
      const catchesPasteKey = /Paste\.isPasteKey\(event\.key,\s*event\.modifiers\)/.test(f.block);
      const forcesNativePaste = /root\.routeKey\([^)]*,\s*true\)/.test(f.block);
      if (!catchesPasteKey || !forcesNativePaste) return true;

      // The interception must run ahead of the plain, bindings-based
      // routeKey fallback textually, so a key bound to something else
      // (or a field.paste/compose.attach moved off Ctrl+V entirely in
      // keys.conf) can never shadow it.
      const interceptIndex = f.block.search(/Paste\.isPasteKey\(/);
      const fallbackIndex = f.block.search(/root\.routeKey\(event\.key,\s*event\.modifiers,\s*event\.text\)\)/);
      return fallbackIndex !== -1 && interceptIndex > fallbackIndex;
    })
    .map((f) => `${f.file} (${f.objectName || 'unnamed'})`);

  assert.deepEqual(offenders, [],
    `${offenders.length} field(s) do not catch a native paste key (ui/lib/Paste.js's own isPasteKey, which ` +
    `covers Ctrl+Shift+Insert and the dedicated Paste key on top of Ctrl+V and Shift+Insert) ahead of routeKey's ` +
    `binding lookup, so Qt's unbounded native paste() still runs whenever that lookup matches nothing or ` +
    `keys.conf has rebound field.paste or compose.attach:\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught: a field with no isPasteKey interception fails', () => {
  const src = 'Item {\n  Ui.TextField {\n    objectName: "x"\n    Keys.onPressed: event => {\n' +
    '      if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true\n    }\n  }\n}\n';
  const match = [...src.matchAll(FIELD_PATTERN)][0];
  const block = blockFor(src, match.index + match[0].length - 1);

  assert.equal(/Paste\.isPasteKey\(/.test(block), false);
});

test('a deliberately broken expectation is caught: a field missing maximumLength fails', () => {
  const src = 'Item {\n  Ui.TextField {\n    objectName: "x"\n    ContextMenu.menu: null\n    MiddleClickGuard {}\n  }\n}\n';
  const match = [...src.matchAll(FIELD_PATTERN)][0];
  const block = blockFor(src, match.index + match[0].length - 1);

  assert.equal(/\bmaximumLength\s*:/.test(block), false);
});

test('the composer\'s own field is found and recognised as the documented exception', () => {
  const composerField = fields().find((f) => f.file === COMPOSER_FIELD.file && f.objectName === COMPOSER_FIELD.objectName);

  assert.ok(composerField, 'Composer.qml\'s own composerInput field was not found at all');
  assert.equal(isComposerField(composerField), true);
});
