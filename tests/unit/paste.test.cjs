// Tests for ui/lib/Paste.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load } = require('./load.cjs');

const Paste = load('lib/Paste.js');
const { KEY, MOD, isPasteKey } = Paste;

// letter returns the Qt key code for a single ASCII letter, the same
// helper tests/unit/keymap.test.cjs uses for the same reason: Qt uses
// the letter's own uppercase ASCII code as its key code.
const letter = (ch) => ch.toUpperCase().charCodeAt(0);

test('forSingleLine leaves a plain one-line string unchanged', () => {
  assert.equal(Paste.forSingleLine('hello there'), 'hello there');
});

test('forSingleLine collapses a Unix newline to a space', () => {
  assert.equal(Paste.forSingleLine('line one\nline two'), 'line one line two');
});

test('forSingleLine collapses a Windows line ending to one space, not two', () => {
  assert.equal(Paste.forSingleLine('line one\r\nline two'), 'line one line two');
});

test('forSingleLine collapses a run of several line breaks to one space', () => {
  assert.equal(Paste.forSingleLine('a\n\n\nb'), 'a b');
});

test('forSingleLine collapses a bare carriage return too', () => {
  assert.equal(Paste.forSingleLine('a\rb'), 'a b');
});

test('forSingleLine returns an empty string unchanged', () => {
  assert.equal(Paste.forSingleLine(''), '');
});

test('forSingleLine leaves leading and trailing line breaks as single spaces, not trimmed away', () => {
  assert.equal(Paste.forSingleLine('\nhello\n'), ' hello ');
});

// Each row mirrors one line of qtbase's own static key-binding table
// for QKeySequence::Paste (qtbase/src/gui/kernel/qplatformtheme.cpp,
// see docs/decisions.md): every key some keyboard scheme resolves to
// Paste, each expected true, plus keys that must NOT be mistaken for
// one - a bare Insert (only toggles overwrite mode), Ctrl+Insert (every
// scheme's own Copy instead), and chords no scheme binds to anything.
const pasteKeyCases = [
  ['Ctrl+V, the one binding every scheme shares', KEY.V, MOD.Ctrl, true],
  ['Shift+Insert, the Windows and X11/Wayland schemes', KEY.Insert, MOD.Shift, true],
  ['Ctrl+Shift+Insert, the X11/Wayland scheme (also active under KDE and GNOME)', KEY.Insert, MOD.Ctrl | MOD.Shift, true],
  ['the dedicated Paste key, every scheme', KEY.Paste, 0, true],
  ["F18, a Sun keyboard's own Paste key under the X11/Wayland scheme", KEY.F18, 0, true],
  ["Meta+Y, macOS's own Emacs-style binding", KEY.Y, MOD.Meta, true],

  ['a bare Insert only toggles overwrite mode', KEY.Insert, 0, false],
  ['Ctrl+Insert is every scheme\'s own Copy, not Paste', KEY.Insert, MOD.Ctrl, false],
  ['a bare V is just typing the letter', KEY.V, 0, false],
  ['Shift+V is just typing a capital V', KEY.V, MOD.Shift, false],
  ['Ctrl+Alt+V is a chord no scheme binds to anything', KEY.V, MOD.Ctrl | MOD.Alt, false],
  ['Ctrl+Y with no Meta is an unrelated chord', KEY.Y, MOD.Ctrl, false],
  ['a bare Y is just typing the letter', KEY.Y, 0, false],
  ['Ctrl+C is Copy, not Paste', letter('c'), MOD.Ctrl, false]
];

for (const [label, key, modifiers, want] of pasteKeyCases) {
  test(`isPasteKey: ${label}`, () => {
    assert.equal(isPasteKey(key, modifiers), want);
  });
}
