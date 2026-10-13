// Tests for ui/lib/SpellCheck.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const SpellCheck = load('lib/SpellCheck.js');

const misspellings = [{ start: 0, end: 5, word: 'hello' }, { start: 9, end: 14, word: 'wrold' }];

const atCaretCases = [
  ['caret inside the first range', 2, misspellings[0]],
  ['caret at a range start', 9, misspellings[1]],
  ['caret at a range end', 14, misspellings[1]],
  ['caret between the two ranges', 7, null],
  ['caret past every range', 99, null]
];

for (const [name, caret, want] of atCaretCases) {
  test(`misspellingAtCaret ${name}`, () => {
    assert.deepEqual(SpellCheck.misspellingAtCaret(misspellings, caret), want);
  });
}

test('misspellingAtCaret with no misspellings is null', () => {
  assert.equal(SpellCheck.misspellingAtCaret([], 0), null);
  assert.equal(SpellCheck.misspellingAtCaret(null, 0), null);
});

test('misspellingAtPosition treats the end offset as exclusive', () => {
  assert.deepEqual(SpellCheck.misspellingAtPosition(misspellings, 4), misspellings[0]);
  assert.equal(SpellCheck.misspellingAtPosition(misspellings, 5), null);
  assert.deepEqual(SpellCheck.misspellingAtPosition(misspellings, 9), misspellings[1]);
});

test('applySuggestion replaces the misspelled range and places the caret right after it', () => {
  const wrold = { start: 10, end: 15, word: 'wrold' };
  const result = SpellCheck.applySuggestion('say hello wrold now', wrold, 'world');
  assert.equal(result.text, 'say hello world now');
  assert.equal(result.cursor, 15);
});

test('applySuggestion with a shorter replacement still places the caret right after it', () => {
  const result = SpellCheck.applySuggestion('hello', { start: 0, end: 5, word: 'hello' }, 'hi');
  assert.equal(result.text, 'hi');
  assert.equal(result.cursor, 2);
});

test('shiftMisspellings leaves a range entirely before the edit alone', () => {
  const got = SpellCheck.shiftMisspellings(misspellings, 20, 3);
  assert.deepEqual(got, misspellings);
});

test('shiftMisspellings shifts a range entirely after an insertion', () => {
  const got = SpellCheck.shiftMisspellings(misspellings, 0, 3);
  assert.deepEqual(got, [{ start: 3, end: 8, word: 'hello' }, { start: 12, end: 17, word: 'wrold' }]);
});

test('shiftMisspellings shifts a range entirely after a deletion', () => {
  const got = SpellCheck.shiftMisspellings(misspellings, 0, -2);
  assert.deepEqual(got, [{ start: -2, end: 3, word: 'hello' }, { start: 7, end: 12, word: 'wrold' }]);
});

test('shiftMisspellings drops a range the edit falls inside of', () => {
  const got = SpellCheck.shiftMisspellings(misspellings, 2, 1);
  assert.deepEqual(got, [{ start: 10, end: 15, word: 'wrold' }]);
});

test('shiftMisspellings with no length change returns the list unchanged', () => {
  assert.deepEqual(SpellCheck.shiftMisspellings(misspellings, 2, 0), misspellings);
});

test('shiftMisspellings with no misspellings returns an empty list', () => {
  assert.deepEqual(SpellCheck.shiftMisspellings(null, 0, 1), []);
});

test('DEBOUNCE_MS is a positive number of milliseconds', () => {
  assert.equal(typeof SpellCheck.DEBOUNCE_MS, 'number');
  assert.ok(SpellCheck.DEBOUNCE_MS > 0);
});
