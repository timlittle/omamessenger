'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Palette = load('lib/Palette.js');

const items = ['New message', 'Next unread conversation', 'Show WhatsApp', 'Mute or unmute chat', 'Quit OmaMessenger'];
const titles = (list) => list.map((s) => s);

test('an empty query keeps every item in order', () => {
  assert.deepEqual(titles(Palette.search(items, '', (s) => s)), items);
});

test('matches letters in order, ignoring case and gaps', () => {
  assert.deepEqual(titles(Palette.search(items, 'nwmsg', (s) => s)), ['New message']);
  assert.deepEqual(titles(Palette.search(items, 'WHATS', (s) => s)), ['Show WhatsApp']);
});

test('drops items that do not contain the letters in order', () => {
  assert.deepEqual(titles(Palette.search(items, 'zzz', (s) => s)), []);
  assert.deepEqual(titles(Palette.search(items, 'gsm', (s) => s)), []);
});

test('ranks word starts and runs of letters first', () => {
  const ranked = Palette.search(['Mute or unmute chat', 'Next unread conversation', 'New message'], 'ne', (s) => s);

  assert.deepEqual(ranked.slice(0, 2), ['Next unread conversation', 'New message']);
  assert.strictEqual(ranked[2], 'Mute or unmute chat');
});

test('searches whatever text the caller picks', () => {
  const rows = [{ title: 'Alex Chen', id: 'a' }, { title: 'Mum', id: 'b' }];

  assert.deepEqual(Palette.search(rows, 'mu', (r) => r.title).map((r) => r.id), ['b']);
});

test('a title containing the whole query beats one matching letter by letter', () => {
  const ranked = Palette.search(['Saved Messages', 'Sam (spotty signal)'], 'sam', (s) => s);

  assert.deepEqual(ranked, ['Sam (spotty signal)', 'Saved Messages']);
});
