// Tests for ui/lib/Format.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load } = require('./load.cjs');

const Format = load('lib/Format.js');

// Fixed nowMs for timezone-independent tests
// 2026-10-06 12:00:00 UTC (but we use local time in Date constructor)
const nowMs = new Date(2026, 9, 6, 12, 0).getTime();

const escapeHtmlCases = [
  ['escapes ampersand', 'a&b', 'a&amp;b'],
  ['escapes less-than', 'a<b', 'a&lt;b'],
  ['escapes double-quote', 'a"b', 'a&quot;b'],
  ['escapes multiple characters', '<div class="test">', '&lt;div class=&quot;test&quot;&gt;']
];

for (const [name, input, want] of escapeHtmlCases) {
  test(`escapeHtml ${name}`, () => {
    assert.equal(Format.escapeHtml(input), want);
  });
}

// linkify turns a bare URL into a clickable link; these cases share the
// same regex-match shape across several URL contexts.
const linkifyCases = [
  ['turns http URLs into links', 'Visit http://example.com for more', /<a href="http:\/\/example\.com">http:\/\/example\.com<\/a>/],
  ['turns https URLs into links', 'Visit https://example.com for more', /<a href="https:\/\/example\.com">https:\/\/example\.com<\/a>/],
  ['handles URL with &amp; entity inside', 'https://example.com?a=1&amp;b=2', /<a href="https:\/\/example\.com\?a=1&amp;b=2">/],
  ['strips trailing period from URL', 'see https://example.com/tickets.', /<a href="https:\/\/example\.com\/tickets">https:\/\/example\.com\/tickets<\/a>\./],
  ['strips trailing comma from URL', 'visit https://example.com,', /<a href="https:\/\/example\.com">https:\/\/example\.com<\/a>,/],
  ['strips trailing parenthesis from URL', 'check (https://example.com)', /check \(<a href="https:\/\/example\.com">https:\/\/example\.com<\/a>\)/]
];

for (const [name, input, want] of linkifyCases) {
  test(`linkify ${name}`, () => {
    assert.match(Format.linkify(input), want);
  });
}

test('linkify does not link plain text without http(s)', () => {
  assert.equal(Format.linkify('Visit example.com for more'), 'Visit example.com for more');
});

test('linkify handles multiple URLs', () => {
  var result = Format.linkify('http://one.com and https://two.com');
  var oneCount = (result.match(/<a href/g) || []).length;
  assert.equal(oneCount, 2);
});

const highlightUnchangedCases = [
  ['does not match inside entities', 'test &amp; demo', 'amp', 'test &amp; demo'],
  ['with empty query returns unchanged', 'Hello World', '', 'Hello World']
];

for (const [name, text, query, want] of highlightUnchangedCases) {
  test(`highlight ${name}`, () => {
    assert.equal(Format.highlight(text, query), want);
  });
}

test('highlight wraps case-insensitive matches in <b>', () => {
  assert.match(Format.highlight('Hello WORLD', 'world'), /<b>WORLD<\/b>/);
});

test('highlight handles multiple matches', () => {
  var result = Format.highlight('apple application apple', 'apple');
  var matches = (result.match(/<b>apple<\/b>/g) || []).length;
  assert.equal(matches, 2);
});

// initials picks the letters a conversation's avatar shows; these cases
// cover plain names, edge cases and names with bracketed text or punctuation.
const initialsCases = [
  ['returns first two letters uppercase', 'Alice Bob', 'AB'],
  ['handles single letter names', 'Alice', 'A'],
  ['handles empty string', '', '?'],
  ['handles only whitespace', '   ', '?'],
  ['handles multi-byte characters', 'José Maria', 'JM'],
  ['ignores bracketed text', 'Sam (spotty signal)', 'S'],
  ['ignores a trailing role in brackets', 'Jordan (Manager)', 'J'],
  ['still finds two initials past a long bracketed suffix', 'Dr. Bartholomew Featherstonehaugh-Wainwright (Dentist)', 'DB'],
  ['keeps a flat number as a second initial', 'Flat 4B', 'F4'],
  ['falls back to the bracketed word when nothing precedes it', '(Notes)', 'N'],
  ['falls back to ? when there is no letter at all', '!!', '?']
];

for (const [name, input, want] of initialsCases) {
  test(`initials ${name}`, () => {
    assert.strictEqual(Format.initials(input), want);
  });
}

test('initials handles emoji safely', () => {
  var result = Format.initials('😀😁');
  assert.equal(result.length, 2);
});

const timeLabelCases = [
  ['returns HH:mm for today', new Date(2026, 9, 6, 14, 30), '14:30'],
  ['returns Yesterday for yesterday', new Date(2026, 9, 5, 14, 30), 'Yesterday'],
  ['returns weekday for dates within 6 days', new Date(2026, 9, 4, 14, 30), /^(Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday)$/],
  ['returns d MMM for older dates this year', new Date(2026, 4, 15, 14, 30), /^15 May$/],
  ['returns d MMM yyyy for another year', new Date(2025, 4, 15, 14, 30), /^15 May 2025$/]
];

for (const [name, date, want] of timeLabelCases) {
  test(`timeLabel ${name}`, () => {
    const result = Format.timeLabel(date.getTime(), nowMs);
    if (want instanceof RegExp) assert.match(result, want);
    else assert.equal(result, want);
  });
}

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

const dayLabelCases = [
  ['returns Today for today', new Date(2026, 9, 6, 14, 30), 'Today'],
  ['returns Yesterday for yesterday', new Date(2026, 9, 5, 14, 30), 'Yesterday'],
  ['returns weekday name within 6 days', new Date(2026, 9, 4, 14, 30), /^(Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday)$/],
  ['returns d MMMM for older dates this year', new Date(2026, 4, 15, 14, 30), /^15 May$/],
  ['returns d MMMM yyyy for another year', new Date(2025, 4, 15, 14, 30), /^15 May 2025$/]
];

for (const [name, date, want] of dayLabelCases) {
  test(`dayLabel ${name}`, () => {
    const result = Format.dayLabel(date.getTime(), nowMs);
    if (want instanceof RegExp) assert.match(result, want);
    else assert.equal(result, want);
  });
}

const statusGlyphCases = [
  ['pending', '○'],
  ['sent', '✓'],
  ['delivered', '✓✓'],
  ['read', '✓✓'],
  ['failed', '!'],
  ['unknown', '']
];

for (const [status, want] of statusGlyphCases) {
  test(`statusGlyph returns ${want || 'empty string'} for ${status}`, () => {
    assert.equal(Format.statusGlyph(status), want);
  });
}

const previewLineCases = [
  ['returns text for direct message',
    { kind: 'direct', preview: 'Hello there', previewSender: 'Alice', previewOutgoing: false },
    'Hello there'],
  ['returns sender: text for group incoming',
    { kind: 'group', preview: 'Hello team', previewSender: 'Alice', previewOutgoing: false },
    'Alice: Hello team'],
  ['returns You: text for group outgoing',
    { kind: 'group', preview: 'I agree', previewSender: 'Me', previewOutgoing: true },
    'You: I agree'],
  ['returns empty string when preview is empty',
    { kind: 'direct', preview: '', previewSender: 'Alice', previewOutgoing: false },
    ''],
  ['returns empty string when preview is null',
    { kind: 'group', preview: null, previewSender: 'Alice', previewOutgoing: false },
    ''],
  ['returns preview text when group has no previewSender',
    { kind: 'group', preview: 'Hello team', previewSender: '', previewOutgoing: false },
    'Hello team'],
  ['returns preview text when group has null previewSender',
    { kind: 'group', preview: 'Hello team', previewSender: null, previewOutgoing: false },
    'Hello team']
];

for (const [name, conv, want] of previewLineCases) {
  test(`previewLine ${name}`, () => {
    assert.equal(Format.previewLine(conv), want);
  });
}

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

test('messageHtml bolds a mention at its offset', () => {
  const html = Format.messageHtml('hi @Nadia how are you', null, null, [{ offset: 3, length: 6 }]);
  assert.equal(html, 'hi <b>@Nadia</b> how are you');
});

test('messageHtml bolds more than one mention, in any order given', () => {
  const html = Format.messageHtml('@Bob and @Alice', null, null, [
    { offset: 9, length: 6 },
    { offset: 0, length: 4 }
  ]);
  assert.equal(html, '<b>@Bob</b> and <b>@Alice</b>');
});

test('messageHtml escapes and links around a mention', () => {
  const html = Format.messageHtml('<b> @Nadia https://x.io', '#89b4fa', null, [{ offset: 4, length: 6 }]);
  assert.equal(html, '&lt;b&gt; <b>@Nadia</b> <a href="https://x.io" style="color:#89b4fa">https://x.io</a>');
});

test('messageHtml skips a mention that no longer fits the text', () => {
  const html = Format.messageHtml('hi', null, null, [{ offset: 3, length: 6 }]);
  assert.equal(html, 'hi');
});

test('messageHtml ignores overlapping mentions, keeping the earlier one', () => {
  const html = Format.messageHtml('@Alice', null, null, [
    { offset: 0, length: 6 },
    { offset: 2, length: 4 }
  ]);
  assert.equal(html, '<b>@Alice</b>');
});

test('messageHtml with no mentions renders exactly as before', () => {
  assert.equal(Format.messageHtml('plain text', null, null, []), 'plain text');
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

test('caption drops the label a photo, video, file or voice note stands in for', () => {
  assert.strictEqual(Format.caption('[Photo]', { kind: 'photo' }), '');
  assert.strictEqual(Format.caption('[Video]', { kind: 'video' }), '');
  assert.strictEqual(Format.caption('[File]', { kind: 'file' }), '');
  assert.strictEqual(Format.caption('[Voice message]', { kind: 'voice' }), '');
  assert.strictEqual(Format.caption('Sunset', { kind: 'photo' }), 'Sunset');
  assert.strictEqual(Format.caption('[Photo]', null), '[Photo]');
  assert.strictEqual(Format.caption('see x.io', { kind: 'link' }), 'see x.io');
});

const fileSizeCases = [
  [0, ''],
  [512, '512 B'],
  [2048, '2.0 KB'],
  [5 * 1024 * 1024, '5.0 MB'],
  [3 * 1024 * 1024 * 1024, '3.0 GB']
];

for (const [bytes, want] of fileSizeCases) {
  test(`fileSize reads ${want || 'nothing'} for ${bytes} bytes`, () => {
    assert.strictEqual(Format.fileSize(bytes), want);
  });
}

const durationCases = [
  [0, ''],
  [5, '0:05'],
  [65, '1:05'],
  [3725, '1:02:05']
];

for (const [secs, want] of durationCases) {
  test(`duration shows ${want || 'nothing'} for ${secs}s`, () => {
    assert.strictEqual(Format.duration(secs), want);
  });
}

// elapsed shows the same clock as duration, but never blank at zero.
const elapsedCases = [
  [0, '0:00'],
  [5, '0:05'],
  [65, '1:05'],
  [3725, '1:02:05'],
  [-3, '0:00']
];

for (const [secs, want] of elapsedCases) {
  test(`elapsed shows ${want} for ${secs}s`, () => {
    assert.strictEqual(Format.elapsed(secs), want);
  });
}

const unreadLabelCases = [
  [0, '0'],
  [7, '7'],
  [99, '99'],
  [100, '99+'],
  [1234, '99+']
];

for (const [count, want] of unreadLabelCases) {
  test(`unreadLabel shows ${want} for ${count}`, () => {
    assert.strictEqual(Format.unreadLabel(count), want);
  });
}

const photoSizeCases = [
  [1920, 1080, '1920 × 1080'],
  [0, 0, ''],
  [100, 0, ''],
  [0, 100, '']
];

for (const [w, h, want] of photoSizeCases) {
  test(`photoSize shows ${want || 'nothing'} for ${w}x${h}`, () => {
    assert.strictEqual(Format.photoSize(w, h), want);
  });
}

const baseNameCases = [
  ['/home/tim/photo.png', 'photo.png'],
  ['report.pdf', 'report.pdf']
];

for (const [path, want] of baseNameCases) {
  test(`baseName keeps ${want} from ${path}`, () => {
    assert.strictEqual(Format.baseName(path), want);
  });
}

const guessMediaKindCases = [
  ['/tmp/photo.PNG', 'photo'],
  ['/tmp/photo.jpg', 'photo'],
  ['/tmp/clip.mp4', 'video'],
  ['/tmp/notes.txt', 'file'],
  ['/tmp/noextension', 'file']
];

for (const [path, want] of guessMediaKindCases) {
  test(`guessMediaKind returns ${want} for ${path}`, () => {
    assert.strictEqual(Format.guessMediaKind(path), want);
  });
}

const mediaFailureReasonCases = [
  ['not-found', 'no longer available'],
  ['expired', 'no longer on the phone'],
  ['download', 'connection problem'],
  ['decrypt', 'could not be verified'],
  ['cache', 'storage problem'],
  ['timeout', 'took too long'],
  ['bogus', ''],
  ['', ''],
  [undefined, '']
];

for (const [reason, want] of mediaFailureReasonCases) {
  const label = reason === undefined ? 'a missing reason' : JSON.stringify(reason);
  test(`mediaFailureReason turns ${label} into ${want || 'nothing'}`, () => {
    assert.strictEqual(Format.mediaFailureReason(reason), want);
  });
}

const snoozeUntilLabelCases = [
  ['shows just the clock time for later today', new Date(2026, 9, 6, 18, 0), '18:00'],
  ['names tomorrow', new Date(2026, 9, 7, 9, 0), 'Tomorrow 09:00'],
  ['names a weekday within six days', new Date(2026, 9, 10, 9, 0), 'Saturday 09:00'],
  ['shows day and month for further-out dates this year', new Date(2026, 9, 27, 9, 0), '27 Oct 09:00'],
  ['includes the year for a date in another year', new Date(2027, 1, 3, 9, 0), '3 Feb 2027 09:00']
];

for (const [name, date, want] of snoozeUntilLabelCases) {
  test(`snoozeUntilLabel ${name}`, () => {
    assert.strictEqual(Format.snoozeUntilLabel(date.getTime(), nowMs), want);
  });
}

const retryingLabelCases = [
  ['counts minutes under an hour away', nowMs + 2 * 60 * 1000, 'Retrying in 2 min'],
  ['rounds a sub-minute wait up to one minute', nowMs + 30 * 1000, 'Retrying in 1 min'],
  ['shows the clock time an hour or more away', new Date(2026, 9, 6, 14, 5).getTime(), 'Retrying at 14:05'],
  ['shows an ellipsis once the retry is due now', nowMs, 'Retrying…'],
  ['shows an ellipsis once the retry is overdue', nowMs - 5000, 'Retrying…']
];

for (const [name, at, want] of retryingLabelCases) {
  test(`retryingLabel ${name}`, () => {
    assert.strictEqual(Format.retryingLabel(at, nowMs), want);
  });
}
