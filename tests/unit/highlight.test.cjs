// Tests for ui/lib/Highlight.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Highlight = load('lib/Highlight.js');

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
  assert.deepEqual(Highlight.links(linked), ['https://text.example/one']);
});

test('links finds every URL in the text, in order, trimming trailing punctuation', () => {
  const texted = message({ text: 'see https://a.example/x. then (https://b.example/y) too' });
  assert.deepEqual(Highlight.links(texted), ['https://a.example/x', 'https://b.example/y']);
});

test('links returns an empty list for a message with no link', () => {
  assert.deepEqual(Highlight.links(message({ text: 'nothing here' })), []);
  assert.deepEqual(Highlight.links(message({ media: { kind: 'photo', width: 1, height: 1 } })), []);
});


test('primaryLink returns the first link, or "" for none', () => {
  assert.strictEqual(Highlight.primaryLink(message({ text: 'https://a.example https://b.example' })), 'https://a.example');
  assert.strictEqual(Highlight.primaryLink(message({ text: 'no links' })), '');
});
