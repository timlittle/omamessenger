// Tests for ui/lib/Highlight.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Highlight = load('lib/Highlight.js');
const Format = load('lib/Format.js');

const ids = ['m1', 'm2', 'm3'];

test('older steps toward the end of the array and stops there', () => {
  assert.strictEqual(Highlight.older(ids, 'm1'), 'm2');
  assert.strictEqual(Highlight.older(ids, 'm2'), 'm3');
  assert.strictEqual(Highlight.older(ids, 'm3'), 'm3');
});

test('newer steps toward the start of the array and stops there', () => {
  assert.strictEqual(Highlight.newer(ids, 'm3'), 'm2');
  assert.strictEqual(Highlight.newer(ids, 'm2'), 'm1');
  assert.strictEqual(Highlight.newer(ids, 'm1'), 'm1');
});

test('older and newer fall back to the first id with no current one', () => {
  assert.strictEqual(Highlight.older([], 'm1'), '');
  assert.strictEqual(Highlight.newer(ids, ''), 'm1');
});

test('atOldest is true only for the last id in the array', () => {
  assert.strictEqual(Highlight.atOldest(ids, 'm3'), true);
  assert.strictEqual(Highlight.atOldest(ids, 'm1'), false);
  assert.strictEqual(Highlight.atOldest([], 'm1'), false);
});

test('afterWriting returns the remembered jump target when it is still loaded', () => {
  assert.strictEqual(Highlight.afterWriting(ids, 'm2'), 'm2');
});

test('afterWriting falls back to the newest id when there is no remembered jump', () => {
  assert.strictEqual(Highlight.afterWriting(ids, ''), 'm1');
});

test('afterWriting falls back to the newest id when the remembered jump is no longer loaded', () => {
  assert.strictEqual(Highlight.afterWriting(ids, 'gone'), 'm1');
});

test('afterWriting returns "" when nothing is loaded, jump or no jump', () => {
  assert.strictEqual(Highlight.afterWriting([], 'm2'), '');
  assert.strictEqual(Highlight.afterWriting([], ''), '');
});

const message = (fields) => Object.assign({ outgoing: false, status: 'received' }, fields);

const hintsCases = [
  ['names reply and react for a plain message',
    message({}),
    'r reply · e react · d delete'],
  ['adds Enter open for a message carrying a file',
    message({ media: { kind: 'file', fileName: 'a.pdf', size: 1 } }),
    'r reply · e react · d delete · Enter open'],
  ['adds Enter play, not Enter open, for a voice note',
    message({ media: { kind: 'voice', duration: 12 } }),
    'r reply · e react · d delete · Enter play'],
  ['adds Enter play, not Enter open, for an audio message',
    message({ media: { kind: 'audio', duration: 180, fileName: 'song.mp3' } }),
    'r reply · e react · d delete · Enter play'],
  ['adds copy, save and open-externally for a photo, alongside Enter open',
    message({ media: { kind: 'photo', width: 10, height: 10 } }),
    'r reply · e react · d delete · Enter open · c copy · s save · x open'],
  ['adds Enter play, plus copy, save and open-externally, for a video',
    message({ media: { kind: 'video', width: 10, height: 10, duration: 3 } }),
    'r reply · e react · d delete · Enter play · c copy · s save · x open'],
  ['leaves out copy, save and open-externally for a voice note',
    message({ media: { kind: 'voice', duration: 12 } }),
    'r reply · e react · d delete · Enter play'],
  ['leaves out copy, save and open-externally for a sticker',
    message({ media: { kind: 'sticker' } }),
    'r reply · e react · d delete · Enter open'],
  ['adds v vote, not Enter open, for an open poll',
    message({ media: { kind: 'poll', poll: { question: 'Lunch?', options: [], totalVoters: 0 } } }),
    'r reply · e react · d delete · v vote'],
  ['adds no vote key for a closed poll',
    message({ media: { kind: 'poll', poll: { question: 'Lunch?', options: [], totalVoters: 0, closed: true } } }),
    'r reply · e react · d delete'],
  ['adds t retry only for a failed outgoing message',
    message({ outgoing: true, status: 'failed' }),
    'r reply · e react · d delete · t retry'],
  ['does not add t retry for an incoming message reported as failed',
    message({ outgoing: false, status: 'failed' }),
    'r reply · e react · d delete'],
  ['combines media and a failed retry',
    message({ outgoing: true, status: 'failed', media: { kind: 'file', fileName: 'a.pdf', size: 1 } }),
    'r reply · e react · d delete · Enter open · t retry'],
  // A link preview is still "media" by Timeline.media's own test, so
  // Enter open keeps showing beside it exactly as it already did before
  // o existed; this only adds the new hint, it does not replace that one.
  ['adds o open link for a message with a link preview whose URL its text shows',
    message({ text: 'https://example.com', media: { kind: 'link', url: 'https://example.com', title: 'Example' } }),
    'r reply · e react · d delete · Enter open · o open link'],
  ['offers no open link key for a preview whose URL its text never shows',
    message({ media: { kind: 'link', url: 'https://example.com', title: 'Example' } }),
    'r reply · e react · d delete · Enter open'],
  ['adds o open link for a plain message whose text carries a URL',
    message({ text: 'see https://example.com/path for more' }),
    'r reply · e react · d delete · o open link'],
  ['leaves out o open link for a message with no link',
    message({ text: 'just words' }),
    'r reply · e react · d delete'],
  ['adds p go to quote for a reply',
    message({ replyTo: { remoteId: 'r1', senderName: 'Alex', text: 'hi' } }),
    'r reply · e react · d delete · p go to quote'],
  ['combines a link, a quote and a failed retry, in order',
    message({
      outgoing: true,
      status: 'failed',
      text: 'see https://example.com',
      replyTo: { remoteId: 'r1', senderName: 'Alex', text: 'hi' }
    }),
    'r reply · e react · d delete · o open link · p go to quote · t retry'],
  ['adds E edit for your own already-sent text message',
    message({ outgoing: true, remoteId: 'r1' }),
    'r reply · e react · d delete · E edit'],
  ['does not add E edit for an incoming message, even with a remote id',
    message({ outgoing: false, remoteId: 'r1' }),
    'r reply · e react · d delete'],
  ['does not add E edit for a message that has not reached the service yet',
    message({ outgoing: true, remoteId: '' }),
    'r reply · e react · d delete'],
  ['does not add E edit for a message carrying media',
    message({ outgoing: true, remoteId: 'r1', media: { kind: 'photo', width: 10, height: 10 } }),
    'r reply · e react · d delete · Enter open · c copy · s save · x open'],
  ['combines a link and editable, in order',
    message({ outgoing: true, remoteId: 'r1', text: 'see https://example.com' }),
    'r reply · e react · d delete · o open link · E edit']
];

for (const [name, msg, want] of hintsCases) {
  test(`hints ${name}`, () => {
    assert.strictEqual(Highlight.hints(msg), want);
  });
}

test('links never reaches a link preview URL the message text does not show', () => {
  // The preview's URL is the sender's to set, independently of the text
  // the bubble shows, so the open key only ever reaches visible links.
  const linked = message({ text: 'https://text.example/one', media: { kind: 'link', url: 'https://preview.example/two' } });
  assert.deepEqual(Highlight.links(linked), [{ url: 'https://text.example/one', displayText: 'https://text.example/one', suspicious: false }]);
});

test('links finds every URL in the text, in order, trimming trailing punctuation', () => {
  const texted = message({ text: 'see https://a.example/x. then (https://b.example/y) too' });
  assert.deepEqual(Highlight.links(texted), [
    { url: 'https://a.example/x', displayText: 'https://a.example/x', suspicious: false },
    { url: 'https://b.example/y', displayText: 'https://b.example/y', suspicious: false }
  ]);
});

test('links returns an empty list for a message with no link', () => {
  assert.deepEqual(Highlight.links(message({ text: 'nothing here' })), []);
  assert.deepEqual(Highlight.links(message({ media: { kind: 'photo', width: 1, height: 1 } })), []);
});

test('links stops exactly where the bubble would for a plain autolinked URL', () => {
  // The bubble's own URL_PATTERN (ui/lib/Format.js, run through
  // messageLinkAnchors here) already stops at an unescaped '>', which
  // a browser reads as ending a URL's host and starting a new
  // userinfo@host past it; this is the no-spans, no-mentions case
  // where that alone is enough to agree with the bubble.
  const spoofed = message({ text: 'https://bank.com>@evil.example/login' });
  assert.deepEqual(Highlight.links(spoofed), [{ url: 'https://bank.com', displayText: 'https://bank.com', suspicious: false }]);
});

test('links agrees with the bubble when a formatting span starts at the exact character that would otherwise join two URLs into one', () => {
  // A security review's own example: raw text that reads as one URL,
  // https://bank.example@evil.example/login, whose real authority host
  // (after the last unescaped '@') is evil.example - but a Telegram
  // entity (bold, here) starting exactly at that '@' makes the bubble
  // linkify only the text before it, "https://bank.example" on its
  // own, as a complete, well-formed URL with no further host to hide,
  // then render "@evil.example/login" as separate, inert bold text
  // (see ui/lib/Format.js's styledSegment, which runs lineHtml per cut
  // segment rather than over the whole span-covered text at once).
  // Matching the whole raw text with a single regex pass - the old
  // Format.plainLinks - knew nothing about that boundary and rejoined
  // both halves into the one link the bubble never showed at all,
  // reaching evil.example instead of the single, harmless link the
  // bubble actually rendered (see docs/decisions.md).
  const text = 'https://bank.example@evil.example/login';
  const spans = [{ kind: 'bold', start: text.indexOf('@'), end: text.length }];
  const disguised = message({ text, formatting: spans });
  // Still flagged suspicious: more of what reads as the same URL
  // ("@evil.example/login") does immediately follow the link's own
  // end, even though it renders bold rather than seamlessly, so
  // Links.needsConfirmation always asks before opening it (see
  // Format.continuesURLToken and docs/decisions.md).
  assert.deepEqual(Highlight.links(disguised), [{ url: 'https://bank.example', displayText: 'https://bank.example', suspicious: true }]);
});

test('links agrees with the bubble when a would-be URL spans two Markdown table cells', () => {
  // A GitHub-style pipe table's own cells are linkified one at a time
  // (see ui/lib/Markdown.js's tableHtml/cellHtml): "https://bank.example.com"
  // and "@evil.example/login" are two separate cells here, so the
  // bubble renders only the first, complete and harmless on its own,
  // as a link, and the second - no "http(s)://" of its own - as plain
  // text. A single regex pass over the whole raw text instead - the
  // old Format.plainLinks - matched straight through the "|" cell
  // boundary (not one of URL_PATTERN's own stop characters) and
  // rejoined both halves into one link reaching evil.example that the
  // bubble never rendered at all.
  const tableText = '| https://bank.example.com|@evil.example/login |\n|-|-|\n| a | b |';
  assert.deepEqual(Highlight.links(message({ text: tableText })), [
    { url: 'https://bank.example.com', displayText: 'https://bank.example.com', suspicious: false }
  ]);
});

test('links always equals the bubble\'s own anchors: a property check over many generated messages', () => {
  // Rather than re-deriving Format.messageLinkAnchors' own cases one by
  // one, this holds links() to the one property that actually matters:
  // whatever the bubble would render as a clickable anchor for this
  // exact text, mentions and formatting is exactly what the open key
  // (o) can reach - never more, never less, and never a different
  // visible text for the same url. A future change that reintroduces a
  // second, separately maintained pattern here, even one that passes
  // every case above, would still have to agree across all of these.
  let seed = 7;
  const rand = () => { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x7fffffff; };
  const pick = (choices) => choices[Math.floor(rand() * choices.length)];

  const words = ['see', 'https://a.example/path', 'https://b.example?x=1&y=2', 'plain', '@carol', 'https://c.example.com>@evil.example/x', '|', 'table', 'cell'];
  const schemes = ['https://link.example/one', 'https://link.example/two', 'mailto:a@example.com', 'javascript:alert(1)'];

  for (let i = 0; i < 200; i++) {
    const wordCount = 1 + Math.floor(rand() * 6);
    const text = Array.from({ length: wordCount }, () => pick(words)).join(' ');

    const mentions = rand() < 0.3 && text.length > 2
      ? [{ offset: 0, length: Math.min(2, text.length) }]
      : [];

    const formatting = rand() < 0.3 && text.length > 2
      ? [{ kind: pick(['bold', 'italic', 'link']), start: 0, end: Math.min(1 + Math.floor(rand() * (text.length - 1)), text.length), url: pick(schemes) }]
      : [];

    const got = Highlight.links(message({ text, mentions, formatting }));
    const want = Format.messageLinkAnchors(text, mentions, formatting);
    assert.deepEqual(got, want,
      `case ${i}: text=${JSON.stringify(text)} mentions=${JSON.stringify(mentions)} formatting=${JSON.stringify(formatting)}`);
  }
});

test('primaryLink returns the first link\'s url, or "" for none', () => {
  assert.strictEqual(Highlight.primaryLink(message({ text: 'https://a.example https://b.example' })), 'https://a.example');
  assert.strictEqual(Highlight.primaryLink(message({ text: 'no links' })), '');
});

test('links strips a long run of trailing punctuation in linear time, not quadratic', () => {
  // A regex like /^(.*?)[.,;:!?)]*$/ backtracks the lazy group one
  // character at a time for every length the punctuation class fails
  // to match at, which costs the square of the run's length. A sender
  // can put tens of thousands of punctuation characters after a link
  // with no length limit of its own (message text is only capped once
  // it reaches the helper, well past what this file ever sees in a
  // test), so this must finish quickly however long that run is.
  const adversarial = `http://${'!'.repeat(65000)}A`;
  const start = Date.now();
  const found = Highlight.links(message({ text: adversarial }));
  const elapsedMs = Date.now() - start;

  assert.deepEqual(found, [{ url: adversarial, displayText: adversarial, suspicious: false }]);
  assert.ok(elapsedMs < 2000, `links took ${elapsedMs}ms on an adversarial trailing-punctuation run, want well under 2000ms`);
});
