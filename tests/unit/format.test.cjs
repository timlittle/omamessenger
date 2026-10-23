// Tests for ui/lib/Format.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load } = require('./load.cjs');

const Format = load('lib/Format.js');

// anchorsPlain copies an anchors list's {url, displayText} entries,
// and the list itself, into plain native ones built in this file's own
// realm: the vm context load() runs Format.js in gives every array and
// object it builds a different [[Prototype]], which fails
// deepStrictEqual's own prototype check against a literal written here
// even when every property matches (see tests/unit/load.test.cjs's own
// note on the same gap). Array.from, called on this file's own Array,
// rebuilds the list itself in this realm too - anchors.map would not,
// since it constructs its result through the source array's own realm.
function anchorsPlain(anchors) {
  return Array.from(anchors, (a) => ({ url: a.url, displayText: a.displayText }));
}

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
// same regex-match shape across several URL contexts. The href it
// writes is always a private "oma-link:<index>" (see
// Format.linkHref), never the URL itself (see "resolveClickedLink
// tells apart two spans..." in links.test.cjs for why), so each case
// also checks the real URL came out the other side, in anchors.
const linkifyCases = [
  ['turns http URLs into links', 'Visit http://example.com for more', /<a href="oma-link:0">http:\/\/example\.com<\/a>/, 'http://example.com'],
  ['turns https URLs into links', 'Visit https://example.com for more', /<a href="oma-link:0">https:\/\/example\.com<\/a>/, 'https://example.com'],
  ['handles URL with &amp; entity inside', 'https://example.com?a=1&amp;b=2', /<a href="oma-link:0">https:\/\/example\.com\?a=1&amp;b=2<\/a>/, 'https://example.com?a=1&amp;b=2'],
  ['strips trailing period from URL', 'see https://example.com/tickets.', /<a href="oma-link:0">https:\/\/example\.com\/tickets<\/a>\./, 'https://example.com/tickets'],
  ['strips trailing comma from URL', 'visit https://example.com,', /<a href="oma-link:0">https:\/\/example\.com<\/a>,/, 'https://example.com'],
  ['strips trailing parenthesis from URL', 'check (https://example.com)', /check \(<a href="oma-link:0">https:\/\/example\.com<\/a>\)/, 'https://example.com']
];

for (const [name, input, want, url] of linkifyCases) {
  test(`linkify ${name}`, () => {
    const anchors = [];
    assert.match(Format.linkify(input, null, anchors), want);
    assert.deepStrictEqual(anchorsPlain(anchors), [{ url, displayText: url }]);
  });
}

test('linkHref and linkIndexFromHref round-trip an anchor\'s own position', () => {
  assert.strictEqual(Format.linkHref(0), 'oma-link:0');
  assert.strictEqual(Format.linkHref(7), 'oma-link:7');
  assert.strictEqual(Format.linkIndexFromHref('oma-link:0'), 0);
  assert.strictEqual(Format.linkIndexFromHref('oma-link:7'), 7);
});

const unknownHrefCases = [
  ['a plain URL, never one of this file\'s own hrefs', 'https://example.com'],
  ['a scheme with no digits after it', 'oma-link:'],
  ['a non-numeric suffix', 'oma-link:abc'],
  ['a negative index', 'oma-link:-1'],
  ['missing entirely', null],
  ['missing entirely', undefined]
];

for (const [name, href] of unknownHrefCases) {
  test(`linkIndexFromHref returns -1 for ${name}`, () => {
    assert.strictEqual(Format.linkIndexFromHref(href), -1);
  });
}

test('linkify does not link plain text without http(s)', () => {
  assert.equal(Format.linkify('Visit example.com for more'), 'Visit example.com for more');
});

test('linkify handles multiple URLs, each with its own numbered href', () => {
  const anchors = [];
  const result = Format.linkify('http://one.com and https://two.com', null, anchors);
  assert.match(result, /<a href="oma-link:0">http:\/\/one\.com<\/a>/);
  assert.match(result, /<a href="oma-link:1">https:\/\/two\.com<\/a>/);
  assert.deepStrictEqual(anchorsPlain(anchors), [
    { url: 'http://one.com', displayText: 'http://one.com' },
    { url: 'https://two.com', displayText: 'https://two.com' }
  ]);
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

test('previewLine caps a huge preview to one row\'s worth of text', () => {
  const huge = 'x'.repeat(100000);
  const line = Format.previewLine({ kind: 'group', preview: huge, previewSender: 'Alice', previewOutgoing: false });
  assert.equal(line.length, Format.MAX_PREVIEW_LENGTH);
  assert.ok(line.startsWith('Alice: x'));
});

test('previewText caps a search match the same way', () => {
  assert.equal(Format.previewText('y'.repeat(100000)).length, Format.MAX_PREVIEW_LENGTH);
  assert.equal(Format.previewText('short'), 'short');
  assert.equal(Format.previewText(null), '');
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
  assert.equal(Format.messageHtml('<b>\nhttps://x.io'), '&lt;b&gt;<br><a href="oma-link:0">https://x.io</a>');
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
  assert.equal(html, '&lt;b&gt; <b>@Nadia</b> <a href="oma-link:0" style="color:#89b4fa">https://x.io</a>');
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

test('messageLinkAnchors still finds an auto-linked URL around a mention', () => {
  const text = '@Nadia see https://x.io';
  const mentions = [{ offset: 0, length: 6 }];
  const html = Format.messageHtml(text, null, null, mentions);
  assert.equal(html, '<b>@Nadia</b> see <a href="oma-link:0">https://x.io</a>');
  assert.deepStrictEqual(anchorsPlain(Format.messageLinkAnchors(text, mentions, [])), [{ url: 'https://x.io', displayText: 'https://x.io' }]);
});

// Formatting spans (a connector's links, bold, italic, underline, strike,
// code, pre and spoiler runs) are rendered on top of the escaped text the
// same way mentions are, through messageHtml's fifth argument.

test('messageHtml renders a link span as an anchor with a private href, never its own URL', () => {
  // A span's URL never reaches the href attribute at all any more (see
  // Format.linkHref and messageLinkAnchors): resolving a click by
  // matching an href against a span's URL is exactly what let two
  // spans sharing one URL make a click on either report whichever
  // span a lookup found first (see links.test.cjs).
  const url = 'https://example.com/a?x=1&y="2"';
  const spans = [{ kind: 'link', start: 4, end: 8, url }];
  const html = Format.messageHtml('see link here', null, null, [], spans);
  assert.equal(html, 'see <a href="oma-link:0">link</a> here');
  assert.deepStrictEqual(anchorsPlain(Format.messageLinkAnchors('see link here', [], spans)), [{ url, displayText: 'link' }]);
});

test('messageHtml colours a link span the same way auto-linked URLs are', () => {
  const html = Format.messageHtml('link', '#89b4fa', null, [], [{ kind: 'link', start: 0, end: 4, url: 'https://x.io' }]);
  assert.equal(html, '<a href="oma-link:0" style="color:#89b4fa">link</a>');
});

test('messageHtml drops a link span whose URL scheme is not allowed', () => {
  const html = Format.messageHtml('click here', null, null, [], [{ kind: 'link', start: 0, end: 5, url: 'javascript:alert(1)' }]);
  assert.equal(html, 'click here');
});

const allowedSchemeCases = ['http://x.io', 'https://x.io', 'mailto:a@x.io', 'tg://resolve?domain=x'];
for (const url of allowedSchemeCases) {
  test(`messageHtml allows a link span with scheme ${url.split(':')[0]}`, () => {
    const spans = [{ kind: 'link', start: 0, end: 2, url }];
    const html = Format.messageHtml('go', null, null, [], spans);
    assert.equal(html, '<a href="oma-link:0">go</a>');
    assert.deepStrictEqual(anchorsPlain(Format.messageLinkAnchors('go', [], spans)), [{ url, displayText: 'go' }]);
  });
}

const simpleSpanCases = [
  ['bold', '<b>text</b>'],
  ['italic', '<i>text</i>'],
  ['underline', '<u>text</u>'],
  ['strike', '<s>text</s>']
];
for (const [kind, want] of simpleSpanCases) {
  test(`messageHtml renders a ${kind} span`, () => {
    assert.equal(Format.messageHtml('text', null, null, [], [{ kind, start: 0, end: 4 }]), want);
  });
}

test('messageHtml renders a code span in monospace', () => {
  const html = Format.messageHtml('text', null, null, [], [{ kind: 'code', start: 0, end: 4 }]);
  assert.equal(html, '<span style="font-family:monospace">text</span>');
});

test('messageHtml renders a pre span the same way as code', () => {
  const html = Format.messageHtml('text', null, null, [], [{ kind: 'pre', start: 0, end: 4 }]);
  assert.equal(html, '<span style="font-family:monospace">text</span>');
});

test('messageHtml renders a spoiler span as plain text, with no tag at all', () => {
  assert.equal(Format.messageHtml('secret', null, null, [], [{ kind: 'spoiler', start: 0, end: 6 }]), 'secret');
});

test('messageHtml escapes text inside a formatting span', () => {
  const html = Format.messageHtml('<script>', null, null, [], [{ kind: 'bold', start: 0, end: 8 }]);
  assert.equal(html, '<b>&lt;script&gt;</b>');
});

test('messageHtml nests overlapping spans into well-formed, matching tags', () => {
  const html = Format.messageHtml('bold italic', null, null, [], [
    { kind: 'bold', start: 0, end: 11 },
    { kind: 'italic', start: 5, end: 11 }
  ]);
  assert.equal(html, '<b>bold </b><i><b>italic</b></i>');
});

test('messageHtml nests a bold span inside a link span', () => {
  // The link is cut into two adjacent anchors, one per active style
  // underneath it, rather than one tag that would have to close and
  // reopen mid-way through; each gets its own private href (see
  // Format.linkHref), so the rendered link is still one continuous
  // clickable region, and messageLinkAnchors reports the span's whole
  // own text - not just the fragment under either href - for both.
  const url = 'https://x.io';
  const spans = [
    { kind: 'link', start: 2, end: 11, url },
    { kind: 'bold', start: 2, end: 6 }
  ];
  const html = Format.messageHtml('a bold link', null, null, [], spans);
  assert.equal(html, 'a <a href="oma-link:0"><b>bold</b></a><a href="oma-link:1"> link</a>');
  assert.deepStrictEqual(anchorsPlain(Format.messageLinkAnchors('a bold link', [], spans)), [
    { url, displayText: 'bold link' },
    { url, displayText: 'bold link' }
  ]);
});

test('messageHtml renders sanely when a span overlaps a mention', () => {
  // Cutting at every range's boundary means the stretch where the
  // italic span and the mention overlap gets both tags, and the italic
  // tag reopens for the stretch after the mention ends but the span has
  // not yet: still well-formed, matching tags throughout.
  const html = Format.messageHtml('@Nadia says hi', null, null, [{ offset: 0, length: 6 }], [
    { kind: 'italic', start: 3, end: 8 }
  ]);
  assert.equal(html, '<b>@Na</b><i><b>dia</b></i><i> s</i>ays hi');
});

test('messageHtml drops a span that no longer fits the text', () => {
  assert.equal(Format.messageHtml('hi', null, null, [], [{ kind: 'bold', start: 0, end: 10 }]), 'hi');
});

// MAX_FORMATTING_SPANS and MAX_MENTIONS back a defense-in-depth cap,
// behind the helper's own domain.MaxFormattingSpans and
// domain.MaxMentions caps applied at ingest: a connector bug or an
// older helper could still hand this file an oversized list, and
// rendering must stay bounded either way.

test('messageHtml bounds an oversized span list to MAX_FORMATTING_SPANS, as defense in depth behind the helper\'s own ingest cap', () => {
  const max = Format.MAX_FORMATTING_SPANS;
  const text = 'x'.repeat(max + 20);
  const spans = [];
  for (let i = 0; i < text.length; i++) spans.push({ kind: 'bold', start: i, end: i + 1 });

  const html = Format.messageHtml(text, null, null, [], spans);
  const boldCount = (html.match(/<b>/g) || []).length;
  assert.equal(boldCount, max);
});

test('messageHtml bounds an oversized mention list to MAX_MENTIONS, as defense in depth behind the helper\'s own ingest cap', () => {
  const max = Format.MAX_MENTIONS;
  const text = 'x'.repeat(max + 20);
  const mentions = [];
  for (let i = 0; i < text.length; i++) mentions.push({ offset: i, length: 1 });

  const html = Format.messageHtml(text, null, null, mentions);
  const boldCount = (html.match(/<b>/g) || []).length;
  assert.equal(boldCount, max);
});

test('messageHtml ignores a span kind it does not recognise, showing plain text', () => {
  assert.equal(Format.messageHtml('text', null, null, [], [{ kind: 'blockquote', start: 0, end: 4 }]), 'text');
});

test('messageHtml never produces an <img> tag, however it is given a formatting span', () => {
  const inputs = [
    [{ kind: 'link', start: 0, end: 5, url: 'data:image/png;base64,AAA' }],
    [{ kind: 'link', start: 0, end: 5, url: '<img src=x onerror=alert(1)>' }],
    [{ kind: 'photo', start: 0, end: 5 }]
  ];
  for (const spans of inputs) {
    const html = Format.messageHtml('<img>', null, null, [], spans);
    assert.ok(!html.includes('<img'), `rendered an <img> from ${JSON.stringify(spans)}: ${html}`);
  }
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
    'see <a href="oma-link:0" style="color:#89b4fa">https://x.io</a>');
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
  assert.match(html, /<a href="oma-link:0" style="color:#89b4fa">https:\/\/x\.io<\/a>/);
  assert.deepStrictEqual(anchorsPlain(Format.messageLinkAnchors(text, [], [])), [{ url: 'https://x.io', displayText: 'https://x.io' }]);
});

test('messageHtml does not touch plain text that merely contains a pipe', () => {
  assert.equal(Format.messageHtml('cost | revenue grew'), 'cost | revenue grew');
});

test('messageHtml defaults a table\'s border colour when none is given', () => {
  const text = '| A |\n| --- |\n| 1 |';
  assert.match(Format.messageHtml(text), /border:1px solid #888888/);
});

// MAX_RICH_TEXT_INPUT_LENGTH is not exported; these tests work from the
// same value Format.js itself documents, so a future change to the
// constant only needs updating in one place to keep them accurate.
const MAX_RICH_TEXT_INPUT_LENGTH = 65536;

test('messageHtml leaves text within the length cap untouched', () => {
  const text = 'a'.repeat(MAX_RICH_TEXT_INPUT_LENGTH);
  assert.equal(Format.messageHtml(text), text);
});

test('messageHtml cuts plain text over the length cap before rendering it', () => {
  const text = 'a'.repeat(MAX_RICH_TEXT_INPUT_LENGTH + 1000);
  assert.equal(Format.messageHtml(text), 'a'.repeat(MAX_RICH_TEXT_INPUT_LENGTH));
});

test('messageHtml bounds its own output even for a message built to escape or table-render large', () => {
  // Escaping (each '<' or '>' grows six-fold) and a huge run of table
  // rows both expand their input; the cap on messageHtml's own input
  // bounds the output regardless, to a fixed multiple of that cap.
  const escaping = Format.messageHtml('<script>'.repeat(200000));
  assert.ok(escaping.length <= MAX_RICH_TEXT_INPUT_LENGTH * 6);

  const row = '| ' + 'c'.repeat(100) + ' |\n';
  const tabular = Format.messageHtml('| A |\n| --- |\n' + row.repeat(10000));
  assert.ok(tabular.length <= MAX_RICH_TEXT_INPUT_LENGTH * 6);
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
  ['/home/user/photo.png', 'photo.png'],
  ['report.pdf', 'report.pdf']
];

for (const [path, want] of baseNameCases) {
  test(`baseName keeps ${want} from ${path}`, () => {
    assert.strictEqual(Format.baseName(path), want);
  });
}

test('savedLocationLabel folds the user\'s home directory to "~"', () => {
  assert.strictEqual(Format.savedLocationLabel('/home/alex/Downloads/run.gif', '/home/alex'), 'Saved to ~/Downloads/run.gif');
});

test('savedLocationLabel shows the path as-is when it is not under home', () => {
  assert.strictEqual(Format.savedLocationLabel('/mnt/data/run.gif', '/home/alex'), 'Saved to /mnt/data/run.gif');
});

test('savedLocationLabel shows the path as-is with no home to fold', () => {
  assert.strictEqual(Format.savedLocationLabel('/home/alex/Downloads/run.gif', ''), 'Saved to /home/alex/Downloads/run.gif');
  assert.strictEqual(Format.savedLocationLabel('/home/alex/Downloads/run.gif', undefined), 'Saved to /home/alex/Downloads/run.gif');
});

const mediaFailureReasonCases = [
  ['not-found', 'no longer available'],
  ['expired', 'no longer on the phone'],
  ['download', 'connection problem'],
  ['decrypt', 'could not be verified'],
  ['cache', 'storage problem'],
  ['timeout', 'took too long'],
  ['too-large', 'too large to load automatically — tap to load'],
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

const mediaUnavailableMessageCases = [
  ['photo', 'not-found', 'This photo is no longer available.'],
  ['video', 'not-found', 'This video is no longer available.'],
  ['video', 'expired', 'This video is no longer on the phone.'],
  ['photo', 'decrypt', 'This photo could not be verified.'],
  ['voice', 'not-found', 'This voice message is no longer available.'],
  ['sticker', 'not-found', 'This sticker is no longer available.'],
  ['document', 'not-found', 'This file is no longer available.'],
  ['video', 'download', ''],
  ['video', 'cache', ''],
  ['video', 'timeout', ''],
  ['video', 'bogus', ''],
  ['video', '', '']
];

for (const [kind, reason, want] of mediaUnavailableMessageCases) {
  test(`mediaUnavailableMessage(${kind}, ${JSON.stringify(reason)}) is ${want || 'nothing'}`, () => {
    assert.strictEqual(Format.mediaUnavailableMessage(kind, reason), want);
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
