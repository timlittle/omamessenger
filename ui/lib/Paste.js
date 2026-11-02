.pragma library

// Paste.js shapes clipboard text for a single-line field: the one rule
// ClipboardPaste.qml applies after reading the helper's bounded
// clipboard.readText, kept here, pure, per javascript.md.

// forSingleLine collapses every line break in text to a space, so a
// multi-line clipboard offer pasted into a one-line field never breaks
// its layout or silently drops everything after the first line.
function forSingleLine(text) {
  return text.replace(/[\r\n]+/g, ' ');
}
