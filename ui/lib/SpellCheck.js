.pragma library

// Pure decisions for the composer's English spell check: which flagged
// range (if any) the caret sits inside, and the text and caret position
// after replacing one with a chosen suggestion. Misspelling ranges come
// from the helper's spell.check result ({start, end, word}), in UTF-16
// code units, the same unit a JavaScript string already indexes by (see
// Mentions.js), so none of this needs any unit conversion either.

// DEBOUNCE_MS is how long Composer waits after the text last changed
// before asking the helper to re-check it, so a fast typist is not
// sending a request on every keystroke.
var DEBOUNCE_MS = 300;

// misspellingAtCaret returns the misspelling range the caret sits
// inside (start <= caret <= end), or null when it is not inside any of
// them. A caret exactly at a range's start or end counts as inside it,
// so the shortcut and the right-click both work at either edge of the
// underline, not only strictly between its ends.
function misspellingAtCaret(misspellings, caret) {
  return (misspellings || []).find((m) => caret >= m.start && caret <= m.end) || null;
}

// misspellingAtPosition returns the misspelling range containing a
// clicked or hovered text position pos, for a right-click that does not
// necessarily move the caret first. Unlike misspellingAtCaret, the end
// offset is exclusive, matching how a position between two adjacent
// words resolves to whichever one it falls inside rather than both.
function misspellingAtPosition(misspellings, pos) {
  return (misspellings || []).find((m) => pos >= m.start && pos < m.end) || null;
}

// applySuggestion replaces misspelling's range in text with
// replacement, and returns the new text and a caret position right
// after the replacement, the same way Mentions.insertMention does for
// an inserted mention.
function applySuggestion(text, misspelling, replacement) {
  const newText = text.slice(0, misspelling.start) + replacement + text.slice(misspelling.end);
  return { text: newText, cursor: misspelling.start + replacement.length };
}

// shiftMisspellings adjusts every misspelling's offsets after an edit
// at editStart that changed the text's length by delta (positive for an
// insertion, negative for a deletion), so stale ranges from before the
// edit can still be drawn correctly until the next re-check finishes. A
// range entirely before the edit is untouched; one entirely at or after
// it shifts by delta; one the edit falls inside of is dropped, since it
// no longer names a single intact word.
function shiftMisspellings(misspellings, editStart, delta) {
  if (delta === 0) return misspellings || [];

  return (misspellings || [])
    .filter((m) => editStart <= m.start || editStart >= m.end)
    .map((m) => (editStart >= m.end ? m : { start: m.start + delta, end: m.end + delta, word: m.word }));
}
