// Tests for ui/lib/Paste.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load } = require('./load.cjs');

const Paste = load('lib/Paste.js');

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
