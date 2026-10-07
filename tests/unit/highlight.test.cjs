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

const message = (fields) => Object.assign({ outgoing: false, status: 'received' }, fields);

test('hints names reply and react for a plain message', () => {
  assert.strictEqual(Highlight.hints(message({})), 'r reply · e react');
});

test('hints adds Enter open for a message carrying media', () => {
  const photo = message({ media: { kind: 'photo', width: 10, height: 10 } });
  assert.strictEqual(Highlight.hints(photo), 'r reply · e react · Enter open');
});

test('hints adds Enter play, not Enter open, for a voice note', () => {
  const voice = message({ media: { kind: 'voice', duration: 12 } });
  assert.strictEqual(Highlight.hints(voice), 'r reply · e react · Enter play');
});

test('hints adds t retry only for a failed outgoing message', () => {
  const failed = message({ outgoing: true, status: 'failed' });
  assert.strictEqual(Highlight.hints(failed), 'r reply · e react · t retry');
});

test('hints does not add t retry for an incoming message reported as failed', () => {
  const incoming = message({ outgoing: false, status: 'failed' });
  assert.strictEqual(Highlight.hints(incoming), 'r reply · e react');
});

test('hints combines media and a failed retry', () => {
  const both = message({ outgoing: true, status: 'failed', media: { kind: 'file', fileName: 'a.pdf', size: 1 } });
  assert.strictEqual(Highlight.hints(both), 'r reply · e react · Enter open · t retry');
});

test('hints adds o open link for a message with a link preview', () => {
  // A link preview is still "media" by Timeline.media's own test, so
  // Enter open keeps showing beside it exactly as it already did before
  // o existed; this only adds the new hint, it does not replace that one.
  const linked = message({ media: { kind: 'link', url: 'https://example.com', title: 'Example' } });
  assert.strictEqual(Highlight.hints(linked), 'r reply · e react · Enter open · o open link');
});

test('hints adds o open link for a plain message whose text carries a URL', () => {
  const texted = message({ text: 'see https://example.com/path for more' });
  assert.strictEqual(Highlight.hints(texted), 'r reply · e react · o open link');
});

test('hints leaves out o open link for a message with no link', () => {
  assert.strictEqual(Highlight.hints(message({ text: 'just words' })), 'r reply · e react');
});

test('hints adds p go to quote for a reply', () => {
  const reply = message({ replyTo: { remoteId: 'r1', senderName: 'Alex', text: 'hi' } });
  assert.strictEqual(Highlight.hints(reply), 'r reply · e react · p go to quote');
});

test('hints combines a link, a quote and a failed retry, in order', () => {
  const all = message({
    outgoing: true,
    status: 'failed',
    text: 'see https://example.com',
    replyTo: { remoteId: 'r1', senderName: 'Alex', text: 'hi' }
  });
  assert.strictEqual(Highlight.hints(all), 'r reply · e react · o open link · p go to quote · t retry');
});

test('links prefers the link preview\'s own URL over the text', () => {
  const linked = message({ text: 'https://text.example/one', media: { kind: 'link', url: 'https://preview.example/two' } });
  assert.deepEqual(Highlight.links(linked), ['https://preview.example/two']);
});

test('links finds every URL in the text, in order, trimming trailing punctuation', () => {
  const texted = message({ text: 'see https://a.example/x. then (https://b.example/y) too' });
  assert.deepEqual(Highlight.links(texted), ['https://a.example/x', 'https://b.example/y']);
});

test('links returns an empty list for a message with no link', () => {
  assert.deepEqual(Highlight.links(message({ text: 'nothing here' })), []);
  assert.deepEqual(Highlight.links(message({ media: { kind: 'photo', width: 1, height: 1 } })), []);
});

test('links reads a model row whose media is a JSON string, same as Timeline.media', () => {
  const row = message({ media: JSON.stringify({ kind: 'link', url: 'https://example.com' }) });
  assert.deepEqual(Highlight.links(row), ['https://example.com']);
});

test('primaryLink returns the first link, or "" for none', () => {
  assert.strictEqual(Highlight.primaryLink(message({ text: 'https://a.example https://b.example' })), 'https://a.example');
  assert.strictEqual(Highlight.primaryLink(message({ text: 'no links' })), '');
});
