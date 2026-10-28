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

// MAX_RENDERED_MISSPELLINGS bounds how many misspellings Composer ever
// turns into underline rectangles, matching the helper's own
// app.MaxSpellMisspellings result cap independently (the same two-
// independently-maintained-but-matching pair Format.js's own
// MAX_RICH_TEXT_INPUT_LENGTH describes, since this file has no way to
// import a Go constant). Composer rebuilds its overlay on every
// keystroke and caret move, so this is defence in depth behind the
// helper's own cap for a connector bug, an older helper, or a result
// kept from before the cap existed.
var MAX_RENDERED_MISSPELLINGS = 200;

// MAX_COMPOSER_TEXT_LENGTH bounds how much text the composer's own
// input holds, in Unicode code points, matching domain.MaxTextLength:
// the same cap messages.send enforces on outgoing text and spell.check
// now enforces on what it scans. A pasted clipboard string has no
// length limit of its own (unlike typing, which is naturally slow), so
// the composer must not hold more than a send could ever accept.
var MAX_COMPOSER_TEXT_LENGTH = 4096;

// capComposerText cuts text to MAX_COMPOSER_TEXT_LENGTH Unicode code
// points, counted the way the string iterator already does (so a
// surrogate pair is never split across the cut), leaving anything
// already within the cap untouched.
function capComposerText(text) {
  if (text.length <= MAX_COMPOSER_TEXT_LENGTH) return text;

  const codePoints = Array.from(text);
  if (codePoints.length <= MAX_COMPOSER_TEXT_LENGTH) return text;

  return codePoints.slice(0, MAX_COMPOSER_TEXT_LENGTH).join('');
}

// formatThousands writes a non-negative integer with "," every three
// digits ("4096" -> "4,096"), for truncationNoticeText below: locale-
// free, like every other number this UI formats, so it reads the same
// for everyone rather than depending on the system's own locale.
function formatThousands(n) {
  return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

// truncationNoticeText is the toast Composer shows once its own
// paste-or-type guard actually cuts the draft to limit characters
// (MAX_COMPOSER_TEXT_LENGTH), so a silent cut always comes with a
// word naming the real cap, not a magic number baked into the UI text.
function truncationNoticeText(limit) {
  return `Message cut to the ${formatThousands(limit)}-character limit`;
}

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
