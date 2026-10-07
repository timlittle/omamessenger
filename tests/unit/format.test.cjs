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

test('linkify strips trailing period from URL', () => {
  var result = Format.linkify('see https://example.com/tickets.');
  assert.match(result, /<a href="https:\/\/example\.com\/tickets">https:\/\/example\.com\/tickets<\/a>\./);
});

test('linkify strips trailing comma from URL', () => {
  var result = Format.linkify('visit https://example.com,');
  assert.match(result, /<a href="https:\/\/example\.com">https:\/\/example\.com<\/a>,/);
});

test('linkify strips trailing parenthesis from URL', () => {
  var result = Format.linkify('check (https://example.com)');
  assert.match(result, /check \(<a href="https:\/\/example\.com">https:\/\/example\.com<\/a>\)/);
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

test('timeLabel handles DST boundary correctly (regression)', () => {
  // Europe spring forward: 2026-03-29 01:00 GMT becomes 02:00 BST.
  // At midnight local time, two dates can be 23 hours apart.
  // Message at 2026-03-28 12:00, now at 2026-03-29 12:00
  // The calendar dates are 1 day apart, so it should return "Yesterday"
  // Pin a timezone with DST so the test means the same thing on any machine
  // (CI runs in UTC, which has no DST). Node applies TZ changes immediately.
  var savedTZ = process.env.TZ;
  process.env.TZ = 'Europe/London';
  try {
    // Clocks go forward at 01:00 on 29 March, so the local midnights of
    // 29 and 30 March are only 23 hours apart: the case Math.floor got wrong.
    var messageDate = new Date(2026, 2, 29, 12, 0);  // March 29
    var nowDate = new Date(2026, 2, 30, 12, 0);      // March 30
    var midnightGap = new Date(2026, 2, 30) - new Date(2026, 2, 29);
    assert.equal(midnightGap, 23 * 60 * 60 * 1000, 'the two local midnights must be 23 h apart');
    assert.equal(Format.timeLabel(messageDate.getTime(), nowDate.getTime()), 'Yesterday');
    assert.equal(Format.dayLabel(messageDate.getTime(), nowDate.getTime()), 'Yesterday');
  } finally {
    if (savedTZ === undefined) delete process.env.TZ; else process.env.TZ = savedTZ;
  }
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

test('previewLine returns preview text when group has no previewSender', () => {
  var conv = {
    kind: 'group',
    preview: 'Hello team',
    previewSender: '',
    previewOutgoing: false
  };
  assert.equal(Format.previewLine(conv), 'Hello team');
});

test('previewLine returns preview text when group has null previewSender', () => {
  var conv = {
    kind: 'group',
    preview: 'Hello team',
    previewSender: null,
    previewOutgoing: false
  };
  assert.equal(Format.previewLine(conv), 'Hello team');
});

test('initials ignore bracketed text and punctuation', () => {
  assert.strictEqual(Format.initials('Sam (spotty signal)'), 'S');
  assert.strictEqual(Format.initials('Jordan (Manager)'), 'J');
  assert.strictEqual(Format.initials('Dr. Bartholomew Featherstonehaugh-Wainwright (Dentist)'), 'DB');
  assert.strictEqual(Format.initials('Flat 4B'), 'F4');
  assert.strictEqual(Format.initials('(Notes)'), 'N');
  assert.strictEqual(Format.initials('!!'), '?');
});

test('a conversation with no activity yet has no time label', () => {
  const now = new Date(2026, 9, 6, 12, 0).getTime();

  assert.strictEqual(Format.timeLabel(0, now), '');
  assert.strictEqual(Format.dayLabel(0, now), '');
});

test('messageHtml keeps line breaks, so bullet lists stay on their own lines', () => {
  assert.equal(Format.messageHtml('Plan:\n• one\n• two'), 'Plan:<br>• one<br>• two');
});

test('messageHtml keeps blank lines and runs of spaces', () => {
  assert.equal(Format.messageHtml('a\n\nb   c'), 'a<br><br>b&nbsp;&nbsp; c');
});

test('messageHtml escapes and links before breaking lines', () => {
  assert.equal(Format.messageHtml('<b>\nhttps://x.io'), '&lt;b&gt;<br><a href="https://x.io">https://x.io</a>');
});

test('longestLine picks the widest line to size a bubble by', () => {
  assert.equal(Format.longestLine('short\na much longer line\nmid'), 'a much longer line');
  assert.equal(Format.longestLine(''), '');
});

test('singleLine collapses line breaks and runs of whitespace', () => {
  assert.equal(Format.singleLine('line one\nline two'), 'line one line two');
  assert.equal(Format.singleLine('a   lot   of    space'), 'a lot of space');
  assert.equal(Format.singleLine('  padded  \n\n  '), 'padded');
});

test('messageHtml colours links so they read on the theme', () => {
  assert.equal(Format.messageHtml('see https://x.io', '#89b4fa'),
    'see <a href="https://x.io" style="color:#89b4fa">https://x.io</a>');
});

test('messageHtml renders a pipe table as a real table, not wrapped text', () => {
  const text = '| Job | When |\n| --- | --- |\n| 06:30 check | Daily |';
  const html = Format.messageHtml(text, '#89b4fa', '#444444');
  assert.match(html, /<table[^>]*>/);
  assert.match(html, /<th[^>]*>Job<\/th>/);
  assert.match(html, /<td[^>]*>06:30 check<\/td>/);
  assert.match(html, /border:1px solid #444444/);
});

test('messageHtml keeps text before and after a table as plain rich text', () => {
  const text = 'Here is the schedule:\n| Job | When |\n| --- | --- |\n| run | 06:30 |\nThat is all.';
  const html = Format.messageHtml(text, '#89b4fa', '#444444');
  assert.match(html, /^Here is the schedule:<table/);
  assert.match(html, /<\/table>That is all\.$/);
});

test('messageHtml escapes a table cell\'s markup', () => {
  const text = '| Name |\n| --- |\n| <script>alert(1)</script> |';
  const html = Format.messageHtml(text);
  assert.doesNotMatch(html, /<script>/);
  assert.match(html, /&lt;script&gt;/);
});

test('messageHtml linkifies a URL inside a table cell', () => {
  const text = '| Link |\n| --- |\n| see https://x.io |';
  const html = Format.messageHtml(text, '#89b4fa');
  assert.match(html, /<a href="https:\/\/x\.io" style="color:#89b4fa">https:\/\/x\.io<\/a>/);
});

test('messageHtml does not touch plain text that merely contains a pipe', () => {
  assert.equal(Format.messageHtml('cost | revenue grew'), 'cost | revenue grew');
});

test('messageHtml defaults a table\'s border colour when none is given', () => {
  const text = '| A |\n| --- |\n| 1 |';
  assert.match(Format.messageHtml(text), /border:1px solid #888888/);
});

test('caption drops the label a photo, video or file stands in for', () => {
  assert.strictEqual(Format.caption('[Photo]', { kind: 'photo' }), '');
  assert.strictEqual(Format.caption('[Video]', { kind: 'video' }), '');
  assert.strictEqual(Format.caption('[File]', { kind: 'file' }), '');
  assert.strictEqual(Format.caption('Sunset', { kind: 'photo' }), 'Sunset');
  assert.strictEqual(Format.caption('[Photo]', null), '[Photo]');
  assert.strictEqual(Format.caption('see x.io', { kind: 'link' }), 'see x.io');
});

test('fileSize reads like a file manager', () => {
  assert.strictEqual(Format.fileSize(0), '');
  assert.strictEqual(Format.fileSize(512), '512 B');
  assert.strictEqual(Format.fileSize(2048), '2.0 KB');
  assert.strictEqual(Format.fileSize(5 * 1024 * 1024), '5.0 MB');
  assert.strictEqual(Format.fileSize(3 * 1024 * 1024 * 1024), '3.0 GB');
});

test('duration shows minutes and seconds, and hours when there are some', () => {
  assert.strictEqual(Format.duration(0), '');
  assert.strictEqual(Format.duration(5), '0:05');
  assert.strictEqual(Format.duration(65), '1:05');
  assert.strictEqual(Format.duration(3725), '1:02:05');
});

test('unreadLabel shows the count below 100', () => {
  assert.strictEqual(Format.unreadLabel(0), '0');
  assert.strictEqual(Format.unreadLabel(7), '7');
  assert.strictEqual(Format.unreadLabel(99), '99');
});

test('unreadLabel caps a crowding count at 99+', () => {
  assert.strictEqual(Format.unreadLabel(100), '99+');
  assert.strictEqual(Format.unreadLabel(1234), '99+');
});

test('photoSize shows pixel dimensions', () => {
  assert.strictEqual(Format.photoSize(1920, 1080), '1920 × 1080');
  assert.strictEqual(Format.photoSize(0, 0), '');
  assert.strictEqual(Format.photoSize(100, 0), '');
  assert.strictEqual(Format.photoSize(0, 100), '');
});

test('baseName keeps only the last path segment', () => {
  assert.strictEqual(Format.baseName('/home/tim/photo.png'), 'photo.png');
  assert.strictEqual(Format.baseName('report.pdf'), 'report.pdf');
});

test('guessMediaKind recognizes common image and video extensions', () => {
  assert.strictEqual(Format.guessMediaKind('/tmp/photo.PNG'), 'photo');
  assert.strictEqual(Format.guessMediaKind('/tmp/photo.jpg'), 'photo');
  assert.strictEqual(Format.guessMediaKind('/tmp/clip.mp4'), 'video');
  assert.strictEqual(Format.guessMediaKind('/tmp/notes.txt'), 'file');
  assert.strictEqual(Format.guessMediaKind('/tmp/noextension'), 'file');
});
