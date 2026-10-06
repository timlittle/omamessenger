// Tests for ui/lib/Format.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load } = require('./load.cjs');

const Format = load('lib/Format.js');

// Fixed nowMs for timezone-independent tests
// 2026-10-06 12:00:00 UTC (but we use local time in Date constructor)
const nowMs = new Date(2026, 9, 6, 12, 0).getTime();

test('escapeHtml escapes ampersand', () => {
  assert.equal(Format.escapeHtml('a&b'), 'a&amp;b');
});

test('escapeHtml escapes less-than', () => {
  assert.equal(Format.escapeHtml('a<b'), 'a&lt;b');
});

test('escapeHtml escapes double-quote', () => {
  assert.equal(Format.escapeHtml('a"b'), 'a&quot;b');
});

test('escapeHtml escapes multiple characters', () => {
  assert.equal(Format.escapeHtml('<div class="test">'), '&lt;div class=&quot;test&quot;&gt;');
});

test('linkify turns http URLs into links', () => {
  var result = Format.linkify('Visit http://example.com for more');
  assert.match(result, /<a href="http:\/\/example\.com">http:\/\/example\.com<\/a>/);
});

test('linkify turns https URLs into links', () => {
  var result = Format.linkify('Visit https://example.com for more');
  assert.match(result, /<a href="https:\/\/example\.com">https:\/\/example\.com<\/a>/);
});

test('linkify does not link plain text without http(s)', () => {
  var result = Format.linkify('Visit example.com for more');
  assert.equal(result, 'Visit example.com for more');
});

test('linkify handles URL with &amp; entity inside', () => {
  var escaped = 'https://example.com?a=1&amp;b=2';
  var result = Format.linkify(escaped);
  assert.match(result, /<a href="https:\/\/example\.com\?a=1&amp;b=2">/);
});

test('linkify handles multiple URLs', () => {
  var result = Format.linkify('http://one.com and https://two.com');
  var oneCount = (result.match(/<a href/g) || []).length;
  assert.equal(oneCount, 2);
});

test('highlight wraps case-insensitive matches in <b>', () => {
  var result = Format.highlight('Hello WORLD', 'world');
  assert.match(result, /<b>WORLD<\/b>/);
});

test('highlight does not match inside entities', () => {
  var escaped = 'test &amp; demo';
  var result = Format.highlight(escaped, 'amp');
  assert.equal(result, 'test &amp; demo');
});

test('highlight handles multiple matches', () => {
  var result = Format.highlight('apple application apple', 'apple');
  var matches = (result.match(/<b>apple<\/b>/g) || []).length;
  assert.equal(matches, 2);
});

test('highlight with empty query returns unchanged', () => {
  var text = 'Hello World';
  assert.equal(Format.highlight(text, ''), text);
});

test('initials returns first two letters uppercase', () => {
  assert.equal(Format.initials('Alice Bob'), 'AB');
});

test('initials handles single letter names', () => {
  assert.equal(Format.initials('Alice'), 'A');
});

test('initials handles empty string', () => {
  assert.equal(Format.initials(''), '?');
});

test('initials handles only whitespace', () => {
  assert.equal(Format.initials('   '), '?');
});

test('initials handles emoji safely', () => {
  var result = Format.initials('😀😁');
  assert.equal(result.length, 2);
});

test('initials handles multi-byte characters', () => {
  var result = Format.initials('José Maria');
  assert.equal(result, 'JM');
});

test('timeLabel returns HH:mm for today', () => {
  var today = new Date(2026, 9, 6, 14, 30);
  var result = Format.timeLabel(today.getTime(), nowMs);
  assert.equal(result, '14:30');
});

test('timeLabel returns Yesterday for yesterday', () => {
  var yesterday = new Date(2026, 9, 5, 14, 30);
  var result = Format.timeLabel(yesterday.getTime(), nowMs);
  assert.equal(result, 'Yesterday');
});

test('timeLabel returns weekday for dates within 6 days', () => {
  var twoDaysAgo = new Date(2026, 9, 4, 14, 30);  // Monday
  var result = Format.timeLabel(twoDaysAgo.getTime(), nowMs);
  assert.match(result, /^(Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday)$/);
});

test('timeLabel returns d MMM for older dates this year', () => {
  var inMay = new Date(2026, 4, 15, 14, 30);
  var result = Format.timeLabel(inMay.getTime(), nowMs);
  assert.match(result, /^15 May$/);
});

test('timeLabel returns d MMM yyyy for another year', () => {
  var lastYear = new Date(2025, 4, 15, 14, 30);
  var result = Format.timeLabel(lastYear.getTime(), nowMs);
  assert.match(result, /^15 May 2025$/);
});

test('dayLabel returns Today for today', () => {
  var today = new Date(2026, 9, 6, 14, 30);
  var result = Format.dayLabel(today.getTime(), nowMs);
  assert.equal(result, 'Today');
});

test('dayLabel returns Yesterday for yesterday', () => {
  var yesterday = new Date(2026, 9, 5, 14, 30);
  var result = Format.dayLabel(yesterday.getTime(), nowMs);
  assert.equal(result, 'Yesterday');
});

test('dayLabel returns weekday name within 6 days', () => {
  var twoDaysAgo = new Date(2026, 9, 4, 14, 30);
  var result = Format.dayLabel(twoDaysAgo.getTime(), nowMs);
  assert.match(result, /^(Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday)$/);
});

test('dayLabel returns d MMMM for older dates this year', () => {
  var inMay = new Date(2026, 4, 15, 14, 30);
  var result = Format.dayLabel(inMay.getTime(), nowMs);
  assert.match(result, /^15 May$/);
});

test('dayLabel returns d MMMM yyyy for another year', () => {
  var lastYear = new Date(2025, 4, 15, 14, 30);
  var result = Format.dayLabel(lastYear.getTime(), nowMs);
  assert.match(result, /^15 May 2025$/);
});

test('statusGlyph returns ○ for pending', () => {
  assert.equal(Format.statusGlyph('pending'), '○');
});

test('statusGlyph returns ✓ for sent', () => {
  assert.equal(Format.statusGlyph('sent'), '✓');
});

test('statusGlyph returns ✓✓ for delivered', () => {
  assert.equal(Format.statusGlyph('delivered'), '✓✓');
});

test('statusGlyph returns ✓✓ for read', () => {
  assert.equal(Format.statusGlyph('read'), '✓✓');
});

test('statusGlyph returns ! for failed', () => {
  assert.equal(Format.statusGlyph('failed'), '!');
});

test('statusGlyph returns empty string for unknown status', () => {
  assert.equal(Format.statusGlyph('unknown'), '');
});

test('previewLine returns text for direct message', () => {
  var conv = {
    kind: 'direct',
    preview: 'Hello there',
    previewSender: 'Alice',
    previewOutgoing: false
  };
  assert.equal(Format.previewLine(conv), 'Hello there');
});

test('previewLine returns sender: text for group incoming', () => {
  var conv = {
    kind: 'group',
    preview: 'Hello team',
    previewSender: 'Alice',
    previewOutgoing: false
  };
  assert.equal(Format.previewLine(conv), 'Alice: Hello team');
});

test('previewLine returns You: text for group outgoing', () => {
  var conv = {
    kind: 'group',
    preview: 'I agree',
    previewSender: 'Me',
    previewOutgoing: true
  };
  assert.equal(Format.previewLine(conv), 'You: I agree');
});

test('previewLine returns empty string when preview is empty', () => {
  var conv = {
    kind: 'direct',
    preview: '',
    previewSender: 'Alice',
    previewOutgoing: false
  };
  assert.equal(Format.previewLine(conv), '');
});

test('previewLine returns empty string when preview is null', () => {
  var conv = {
    kind: 'group',
    preview: null,
    previewSender: 'Alice',
    previewOutgoing: false
  };
  assert.equal(Format.previewLine(conv), '');
});
