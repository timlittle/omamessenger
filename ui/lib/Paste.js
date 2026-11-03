.pragma library

// Paste.js shapes clipboard text for a single-line field (forSingleLine,
// the one rule ClipboardPaste.qml applies after reading the helper's
// bounded clipboard.readText) and recognises a native "paste" key press
// without asking Qt (isPasteKey, used by every field that intercepts
// one ahead of its own routeKey binding lookup - see
// docs/decisions.md). Both pure, per javascript.md.

// KEY and MOD hold the Qt key and modifier codes isPasteKey checks,
// defined here for the same reason ui/lib/Keymap.js defines its own:
// node has no Qt, so the node tests need these as plain numbers (see
// qtbase's qnamespace.h for each value). Insert and the Shift/Ctrl
// masks match Keymap.js's own KEY/MOD exactly; the rest - V, Y, F18,
// Paste and the Meta mask - belong only here, since no key binding in
// Keymap.js ever uses them.
var KEY = { V: 0x56, Y: 0x59, Insert: 0x01000006, F18: 0x01000041, Paste: 0x010000e2 };
var MOD = { Shift: 0x02000000, Ctrl: 0x04000000, Alt: 0x08000000, Meta: 0x10000000 };

// forSingleLine collapses every line break in text to a space, so a
// multi-line clipboard offer pasted into a one-line field never breaks
// its layout or silently drops everything after the first line.
function forSingleLine(text) {
  return text.replace(/[\r\n]+/g, ' ');
}

// isPasteKey reports whether a key press is one of Qt's own "paste"
// shortcuts: every key QKeySequence::StandardKey::Paste resolves to
// under some keyboard scheme (Windows, the generic X11/Wayland scheme,
// KDE, GNOME, CDE/Motif or macOS - qtbase's qplatformtheme.cpp has the
// full table), plus the dedicated Paste key every scheme answers to.
// Qt itself only recognises the keys its *active* scheme lists, and
// that scheme comes from QT_QPA_PLATFORMTHEME, which differs between a
// developer's desktop session and a bare CI container (see
// docs/decisions.md): checking this fixed list instead of
// event.matches(StandardKey.Paste) is what makes a paste key
// recognised the same way regardless of which, if any, is set.
// Ctrl+Shift+Insert and F18 (tagged for the X11/Wayland scheme alone)
// are the two this app's own desktop session already answers to that a
// bare CI container does not; Meta+Y (macOS's own Emacs-style binding)
// can never actually arrive on this Linux-only UI but is included for
// the same reason the rest of this list is - a fixed list that quietly
// dropped a scheme would just be a smaller version of the bug it
// exists to fix.
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
