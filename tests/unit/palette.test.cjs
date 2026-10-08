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

test('conversationOrder: puts every unread conversation ahead of read ones', () => {
  const conversations = [
    { id: 'read-new', unread: 0, lastActivity: 300 },
    { id: 'unread-old', unread: 2, lastActivity: 100 },
    { id: 'read-old', unread: 0, lastActivity: 50 },
    { id: 'unread-new', unread: 1, lastActivity: 200 }
  ];

  const order = Palette.conversationOrder(conversations).map((c) => c.id);

  assert.deepEqual(order, ['unread-new', 'unread-old', 'read-new', 'read-old']);
});

test('conversationOrder: orders each group by most recent activity first', () => {
  const unread = [
    { id: 'u1', unread: 1, lastActivity: 100 },
    { id: 'u2', unread: 3, lastActivity: 300 }
  ];
  const read = [
    { id: 'r1', unread: 0, lastActivity: 10 },
    { id: 'r2', unread: 0, lastActivity: 20 }
  ];

  assert.deepEqual(Palette.conversationOrder(unread).map((c) => c.id), ['u2', 'u1']);
  assert.deepEqual(Palette.conversationOrder(read).map((c) => c.id), ['r2', 'r1']);
});

test('conversationOrder: does not mutate its input', () => {
  const conversations = [{ id: 'a', unread: 0, lastActivity: 1 }, { id: 'b', unread: 1, lastActivity: 2 }];
  const copy = conversations.slice();

  Palette.conversationOrder(conversations);

  assert.deepEqual(conversations, copy);
});

test('conversationOrder: handles null or undefined input', () => {
  assert.deepEqual(Palette.conversationOrder(null), []);
  assert.deepEqual(Palette.conversationOrder(undefined), []);
});

test('messageRows: keeps only conversations with a matched message, in search order', () => {
  const conversations = [
    { id: 'a', title: 'Alpha', service: 'whatsapp', match: 'see you then', matchMessageId: 'm1', matchSender: 'Alex' },
    { id: 'b', title: 'Beta', service: 'telegram' }, // title-only match: no matched message
    { id: 'c', title: 'Gamma', service: 'whatsapp', match: 'ok', matchMessageId: 'm3', matchSender: 'Priya' }
  ];

  assert.deepEqual(Palette.messageRows(conversations), [
    { conversationId: 'a', messageId: 'm1', sender: 'Alex', conversationTitle: 'Alpha', service: 'whatsapp', snippet: 'see you then' },
    { conversationId: 'c', messageId: 'm3', sender: 'Priya', conversationTitle: 'Gamma', service: 'whatsapp', snippet: 'ok' }
  ]);
});

test('messageRows: handles null or undefined input', () => {
  assert.deepEqual(Palette.messageRows(null), []);
  assert.deepEqual(Palette.messageRows(undefined), []);
});

test('messageRows: falls back to an empty sender or snippet', () => {
  const conversations = [{ id: 'a', title: 'Alpha', matchMessageId: 'm1' }];

  assert.deepEqual(Palette.messageRows(conversations), [
    { conversationId: 'a', messageId: 'm1', sender: '', conversationTitle: 'Alpha', service: undefined, snippet: '' }
  ]);
});

test('staleMessageSearch: a reply for a query the palette has moved on from is stale', () => {
  assert.strictEqual(Palette.staleMessageSearch('cafe', 'cafe later'), true);
  assert.strictEqual(Palette.staleMessageSearch('cafe', 'cafe'), false);
});
