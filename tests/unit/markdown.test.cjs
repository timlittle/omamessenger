// Tests for ui/lib/Markdown.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Markdown = load('lib/Markdown.js');

// identity stands in for a caller's escape or linkify function in tests
// that do not care about escaping, so the table's shape is what is
// checked instead.
function identity(text) {
  return text;
}

const TABLE_TEXT = [
  '| Job | When |',
  '| --- | --- |',
  '| 06:30 check | Daily |',
].join('\n');

test('splitTables recognizes a header, separator and one body row', () => {
  const parts = Markdown.splitTables(TABLE_TEXT);
  assert.equal(parts.length, 1);
  assert.equal(parts[0].kind, 'table');
  assert.deepEqual(parts[0].table.header, ['Job', 'When']);
  assert.deepEqual(parts[0].table.rows, [['06:30 check', 'Daily']]);
});

test('splitTables keeps text before and after the table as separate parts', () => {
  const text = `before\n${TABLE_TEXT}\nafter`;
  const parts = Markdown.splitTables(text);
  assert.equal(parts.length, 3);
  assert.deepEqual(parts[0], { kind: 'text', text: 'before' });
  assert.equal(parts[1].kind, 'table');
  assert.deepEqual(parts[2], { kind: 'text', text: 'after' });
});

test('splitTables leaves plain text with a stray pipe untouched', () => {
  const parts = Markdown.splitTables('cost | revenue are not a table');
  assert.deepEqual(parts, [{ kind: 'text', text: 'cost | revenue are not a table' }]);
});

test('splitTables leaves a header-like line alone without a matching separator', () => {
  const text = 'Name | Age\nAlex | 30';
  assert.deepEqual(Markdown.splitTables(text), [{ kind: 'text', text }]);
});

test('splitTables rejects a separator row with the wrong column count', () => {
  const text = 'Name | Age\n---\nAlex | 30';
  assert.deepEqual(Markdown.splitTables(text), [{ kind: 'text', text }]);
});

test('splitTables accepts leading and trailing pipes being optional', () => {
  const text = 'Job | When\n--- | ---\n06:30 | Daily';
  const parts = Markdown.splitTables(text);
  assert.equal(parts[0].kind, 'table');
  assert.deepEqual(parts[0].table.header, ['Job', 'When']);
});

test('splitTables pads a ragged row with fewer cells than the header', () => {
  const text = '| A | B | C |\n| - | - | - |\n| 1 | 2 |';
  const parts = Markdown.splitTables(text);
  assert.deepEqual(parts[0].table.rows, [['1', '2', '']]);
});

test('splitTables drops extra cells from a ragged row with more than the header', () => {
  const text = '| A | B |\n| - | - |\n| 1 | 2 | 3 |';
  const parts = Markdown.splitTables(text);
  assert.deepEqual(parts[0].table.rows, [['1', '2']]);
});

test('splitTables ends the table at a blank line', () => {
  const text = `${TABLE_TEXT}\n\nafter the table`;
  const parts = Markdown.splitTables(text);
  assert.equal(parts.length, 2);
  assert.equal(parts[0].kind, 'table');
  assert.deepEqual(parts[1], { kind: 'text', text: '\nafter the table' });
});

test('splitTables ends the table at a line without a pipe', () => {
  const text = `${TABLE_TEXT}\nno pipe here`;
  const parts = Markdown.splitTables(text);
  assert.equal(parts.length, 2);
  assert.deepEqual(parts[1], { kind: 'text', text: 'no pipe here' });
});

test('splitTables reads left, right, center and absent alignment from colons', () => {
  const text = '| A | B | C | D |\n| :-- | --: | :-: | -- |\n| 1 | 2 | 3 | 4 |';
  const parts = Markdown.splitTables(text);
  assert.deepEqual(parts[0].table.aligns, ['left', 'right', 'center', '']);
});

test('tableHtml renders a header row in <th> and body rows in <td>', () => {
  const parts = Markdown.splitTables(TABLE_TEXT);
  const html = Markdown.tableHtml(parts[0].table, identity, identity, '#888888');
  assert.match(html, /<table[^>]*>/);
  assert.match(html, /<th[^>]*>Job<\/th>/);
  assert.match(html, /<th[^>]*>When<\/th>/);
  assert.match(html, /<td[^>]*>06:30 check<\/td>/);
});

test('tableHtml uses the given border color on every cell', () => {
  const parts = Markdown.splitTables(TABLE_TEXT);
  const html = Markdown.tableHtml(parts[0].table, identity, identity, '#ff0000');
  const cellCount = (html.match(/<t[hd]/g) || []).length;
  const borderCount = (html.match(/border:1px solid #ff0000/g) || []).length;
  assert.equal(borderCount, cellCount);
});

test('tableHtml escapes a cell before linkifying it', () => {
  const text = '| Name |\n| --- |\n| <script> |';
  const parts = Markdown.splitTables(text);
  const escapeHtml = (s) => s.replace(/</g, '&lt;').replace(/>/g, '&gt;');
  const html = Markdown.tableHtml(parts[0].table, escapeHtml, identity, '#888888');
  assert.match(html, /&lt;script&gt;/);
  assert.doesNotMatch(html, /<script>/);
});

test('tableHtml runs each cell through the given linkify function', () => {
  const text = '| Link |\n| --- |\n| see https://x.io |';
  const parts = Markdown.splitTables(text);
  const linkify = (s) => s.replace('https://x.io', '<a href="https://x.io">https://x.io</a>');
  const html = Markdown.tableHtml(parts[0].table, identity, linkify, '#888888');
  assert.match(html, /<a href="https:\/\/x\.io">/);
});

test('tableHtml right-aligns a column whose separator ends with a colon', () => {
  const text = '| N |\n| --: |\n| 1 |';
  const parts = Markdown.splitTables(text);
  const html = Markdown.tableHtml(parts[0].table, identity, identity, '#888888');
  assert.match(html, /text-align:right/);
});

test('tableHtml fits the width of whatever holds it', () => {
  const parts = Markdown.splitTables(TABLE_TEXT);
  const html = Markdown.tableHtml(parts[0].table, identity, identity, '#888888');
  assert.match(html, /width="100%"/);
});
