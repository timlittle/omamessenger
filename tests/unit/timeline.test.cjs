// Tests for ui/lib/Timeline.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Timeline = load('lib/Timeline.js');

// Fixed nowMs for consistent day labels: 2026-10-06 12:00:00
const nowMs = new Date(2026, 9, 6, 12, 0).getTime();

function msg(id, created, senderId, outgoing) {
  return { id, senderId, created, outgoing };
}

test('annotate empty list', () => {
  const result = Timeline.annotate([], false, nowMs);
  assert.deepEqual(result, []);
});

test('annotate single message shows day', () => {
  const msgs = [msg('m1', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false)];
  const result = Timeline.annotate(msgs, false, nowMs);
  assert.strictEqual(result.length, 1);
  assert.strictEqual(result[0].showDay, true);
  assert.match(result[0].dayLabel, /Today/);
  assert.strictEqual(result[0].groupedWithOlder, false);
});

test('annotate single message never shows sender in direct chat', () => {
  const msgs = [msg('m1', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false)];
  const result = Timeline.annotate(msgs, false, nowMs);
  assert.strictEqual(result[0].showSender, false);
});

test('annotate single message does not show sender for outgoing in group', () => {
  const msgs = [msg('m1', new Date(2026, 9, 6, 10, 0).getTime(), 'me', true)];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].showSender, false);
});

test('annotate single incoming message shows sender in group', () => {
  const msgs = [msg('m1', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false)];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].showSender, true);
});

test('annotate day separator when dates differ', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 5, 10, 0).getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, false, nowMs);
  assert.strictEqual(result[0].showDay, true);
  assert.match(result[0].dayLabel, /Today/);
  assert.strictEqual(result[1].showDay, true);
  assert.match(result[1].dayLabel, /Yesterday/);
});

test('annotate newer message in group shows sender when older has different sender', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 50).getTime(), 'bob', false)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].showSender, true);
});

test('annotate newer message in group does not show sender when older has same sender', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 50).getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].showSender, false);
});

test('annotate newer message in group shows sender when older is outgoing', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 50).getTime(), 'alice', true)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].showSender, true);
});

test('annotate messages grouped when same sender within 5 minutes', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 57).getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].groupedWithOlder, true);
});

test('annotate messages not grouped when same sender but > 5 minutes apart', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 54).getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].groupedWithOlder, false);
});

test('annotate messages not grouped when different senders', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 57).getTime(), 'bob', false)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].groupedWithOlder, false);
});

test('annotate messages not grouped when different outgoing status', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 57).getTime(), 'alice', true)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].groupedWithOlder, false);
});

test('annotate messages not grouped on day boundary', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 0, 1).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 5, 23, 59).getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].showDay, true);
  assert.strictEqual(result[0].groupedWithOlder, false);
});

test('annotate shows sender when day boundary crossed', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 0, 1).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 5, 23, 59).getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  assert.strictEqual(result[0].showSender, true);
});

test('annotate 5 minute boundary exactly', () => {
  const now = new Date(2026, 9, 6, 10, 0, 0);
  const fiveMinBefore = new Date(2026, 9, 6, 9, 55, 0);
  const msgs = [
    msg('m2', now.getTime(), 'alice', false),
    msg('m1', fiveMinBefore.getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  // Exactly at 5 minutes should NOT be grouped (< 5 min, not <= 5 min)
  assert.strictEqual(result[0].groupedWithOlder, false, 'at exactly 5 min boundary');
});

test('annotate 4:59 within 5 minute boundary', () => {
  const now = new Date(2026, 9, 6, 10, 0);
  const almostFiveBefore = new Date(2026, 9, 6, 9, 55, 1);
  const msgs = [
    msg('m2', now.getTime(), 'alice', false),
    msg('m1', almostFiveBefore.getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, true, nowMs);
  // 4 min 59 sec is less than 5 min, should be grouped
  assert.strictEqual(result[0].groupedWithOlder, true);
});

test('annotate never shows sender in direct chat regardless of time', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 50).getTime(), 'bob', false)
  ];
  const result = Timeline.annotate(msgs, false, nowMs);
  assert.strictEqual(result[0].showSender, false);
  assert.strictEqual(result[1].showSender, false);
});

test('annotate day label empty when no day change', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 50).getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, false, nowMs);
  assert.strictEqual(result[0].dayLabel, '');
  assert.match(result[1].dayLabel, /Today/);
});

test('annotateAt equals annotate for every index in a complex list', () => {
  const msgs = [
    msg('m4', new Date(2026, 9, 6, 14, 0).getTime(), 'alice', false),
    msg('m3', new Date(2026, 9, 6, 13, 55).getTime(), 'alice', false),
    msg('m2', new Date(2026, 9, 5, 20, 0).getTime(), 'bob', true),
    msg('m1', new Date(2026, 9, 5, 10, 0).getTime(), 'bob', true)
  ];

  const annotated = Timeline.annotate(msgs, true, nowMs);

  for (let i = 0; i < msgs.length; i++) {
    const single = Timeline.annotateAt(msgs, i, true, nowMs);
    assert.deepEqual(single, annotated[i], `Mismatch at index ${i}`);
  }
});

test('annotateAt first message has no older neighbour', () => {
  const msgs = [
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 6, 9, 50).getTime(), 'alice', false)
  ];
  const result = Timeline.annotateAt(msgs, 0, true, nowMs);
  assert.strictEqual(result.groupedWithOlder, false);
});

test('annotateAt incoming message shows sender in group when it is the first', () => {
  const msgs = [
    msg('m1', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false)
  ];
  const result = Timeline.annotateAt(msgs, 0, true, nowMs);
  assert.strictEqual(result.showSender, true);
});

test('annotateAt outgoing message never shows sender in group', () => {
  const msgs = [
    msg('m1', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', true)
  ];
  const result = Timeline.annotateAt(msgs, 0, true, nowMs);
  assert.strictEqual(result.showSender, false);
});

test('annotate multi-day conversation with day separators', () => {
  const msgs = [
    msg('m5', new Date(2026, 9, 7, 10, 0).getTime(), 'alice', false),
    msg('m4', new Date(2026, 9, 7, 9, 0).getTime(), 'alice', false),
    msg('m3', new Date(2026, 9, 6, 20, 0).getTime(), 'alice', false),
    msg('m2', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false),
    msg('m1', new Date(2026, 9, 5, 10, 0).getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, false, nowMs);

  // Index 0 (2026-10-07): differs from index 1 (2026-10-07)? No, so no separator
  assert.strictEqual(result[0].showDay, false);
  // Index 1 (2026-10-07): differs from index 2 (2026-10-06)? Yes, show separator
  assert.strictEqual(result[1].showDay, true);
  // Index 2 (2026-10-06): differs from index 3 (2026-10-06)? No, so no separator
  assert.strictEqual(result[2].showDay, false);
  // Index 3 (2026-10-06): differs from index 4 (2026-10-05)? Yes, show separator
  assert.strictEqual(result[3].showDay, true);
  // Index 4 (2026-10-05): is oldest (last index), show separator
  assert.strictEqual(result[4].showDay, true);
});

test('annotate respects different senders in direct chat labels', () => {
  const msgs = [
    msg('m1', new Date(2026, 9, 6, 10, 0).getTime(), 'alice', false)
  ];
  const result = Timeline.annotate(msgs, false, nowMs);
  // Direct chat: never show sender label regardless
  assert.strictEqual(result[0].showSender, false);
});

test('insertIndex places a message by time in a newest-first list', () => {
  const newestFirst = [{ created: 30 }, { created: 20 }, { created: 10 }];

  assert.strictEqual(Timeline.insertIndex(newestFirst, { created: 40 }), 0);
  assert.strictEqual(Timeline.insertIndex(newestFirst, { created: 25 }), 1);
  assert.strictEqual(Timeline.insertIndex(newestFirst, { created: 20 }), 1);
  assert.strictEqual(Timeline.insertIndex(newestFirst, { created: 5 }), 3);
  assert.strictEqual(Timeline.insertIndex([], { created: 5 }), 0);
});

test('row keeps media as a string, so every model row has the same shape', () => {
  const link = { kind: 'link', url: 'https://x.io', title: 'X' };

  assert.strictEqual(Timeline.row({ id: 'a', media: link }).media, JSON.stringify(link));
  assert.strictEqual(Timeline.row({ id: 'b' }).media, '');
  assert.strictEqual(Timeline.row({ id: 'c', media: link }).id, 'c');
});

test('media reads a row\'s media back, or null', () => {
  assert.deepEqual(Timeline.media({ media: '{"kind":"link"}' }), { kind: 'link' });
  assert.strictEqual(Timeline.media({ media: '' }), null);
  assert.deepEqual(Timeline.media({ media: { kind: 'photo' } }), { kind: 'photo' });
});

test('row keeps a downloaded media path, and starts without one', () => {
  assert.strictEqual(Timeline.row({ id: 'a' }).mediaPath, '');
  assert.strictEqual(Timeline.row({ id: 'a', mediaPath: '/m/a.jpg' }).mediaPath, '/m/a.jpg');
});

test('row keeps replyTo as a string, so every model row has the same shape', () => {
  const reply = { remoteId: '1', senderName: 'Alex', text: 'original' };

  assert.strictEqual(Timeline.row({ id: 'a', replyTo: reply }).replyTo, JSON.stringify(reply));
  assert.strictEqual(Timeline.row({ id: 'b' }).replyTo, '');
});

test('replyTo reads a row\'s quoted message back, or null', () => {
  assert.deepEqual(Timeline.replyTo({ replyTo: '{"remoteId":"1"}' }), { remoteId: '1' });
  assert.strictEqual(Timeline.replyTo({ replyTo: '' }), null);
  assert.deepEqual(Timeline.replyTo({ replyTo: { remoteId: '2' } }), { remoteId: '2' });
});

test('row keeps reactions as a string, so updating them later does not silently drop them', () => {
  const reactions = [{ emoji: '👍', count: 1, mine: true }];

  assert.strictEqual(Timeline.row({ id: 'a' }).reactions, '[]');
  assert.strictEqual(Timeline.row({ id: 'a', reactions }).reactions, JSON.stringify(reactions));
});

test('reactions reads a row\'s reaction chips back, or an empty list', () => {
  assert.deepEqual(Timeline.reactions({ reactions: '[{"emoji":"👍","count":1,"mine":true}]' }), [{ emoji: '👍', count: 1, mine: true }]);
  assert.deepEqual(Timeline.reactions({ reactions: '' }), []);
  assert.deepEqual(Timeline.reactions({ reactions: [{ emoji: '❤️', count: 2, mine: false }] }), [{ emoji: '❤️', count: 2, mine: false }]);
  assert.deepEqual(Timeline.reactions({}), []);
});
