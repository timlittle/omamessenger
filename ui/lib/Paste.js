.pragma library

// Paste.js shapes clipboard text for single-line fields and recognises
// native paste key presses without relying on Qt's active theme.

// KEY and MOD are the Qt key/modifier codes isPasteKey checks, defined
// here as plain numbers since node has no Qt (see docs/decisions.md).
var KEY = { V: 0x56, Y: 0x59, Insert: 0x01000006, F18: 0x01000041, Paste: 0x010000e2 };
var MOD = { Shift: 0x02000000, Ctrl: 0x04000000, Alt: 0x08000000, Meta: 0x10000000 };

// forSingleLine collapses line breaks to spaces so a multi-line paste
// doesn't break a single-line field's layout.
function forSingleLine(text) {
  return text.replace(/[\r\n]+/g, ' ');
}

// isPasteKey reports whether a key press is one of Qt's "paste"
// shortcuts across any keyboard scheme, checked directly rather than
// via Qt's active theme, which varies between desktop and CI.
function isPasteKey(key, modifiers) {
  const held = (mask) => (modifiers & mask) !== 0;
  if (held(MOD.Alt)) return false;

  const shift = held(MOD.Shift), ctrl = held(MOD.Ctrl), meta = held(MOD.Meta);
  if (key === KEY.Paste || key === KEY.F18) return !shift && !ctrl && !meta;
  if (key === KEY.Insert) return shift;
  if (key === KEY.V) return ctrl && !meta;
  if (key === KEY.Y) return meta && !ctrl;
  return false;
}
