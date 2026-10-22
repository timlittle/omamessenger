.pragma library

// Pure logic for the confirmation toast (see ui/components/Toast.qml):
// which of two candidate texts wins, and how the winning text is shown
// so its meaning is never colour alone.

// pickText returns hintText while hintActive is true and hintText is
// non-empty, falling back to notice otherwise. WindowController uses
// this to decide whether the one-time "Updated to vX" hint or an
// ordinary confirmation (copying, saving, …) is the toast's current
// text; the hint always wins the one time it has something to say.
function pickText(hintActive, hintText, notice) {
  if (hintActive && hintText) return hintText;
  return notice || '';
}

// displayText pairs text with a leading checkmark glyph, so the toast
// never relies on its accent colour alone to read as good news. Blank
// or empty text stays "": nothing to pair a glyph with, and an empty
// result also means the toast itself should be hidden.
function displayText(text) {
  const trimmed = typeof text === 'string' ? text.trim() : '';
  if (!trimmed) return '';
  return '✓ ' + trimmed;
}
