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

test('readJsonField reads a string field as JSON, an object field as-is, and falls back when absent', () => {
  assert.deepEqual(Timeline.readJsonField({ media: '{"kind":"link"}' }, 'media', null), { kind: 'link' });
  assert.deepEqual(Timeline.readJsonField({ media: { kind: 'photo' } }, 'media', null), { kind: 'photo' });
  assert.strictEqual(Timeline.readJsonField({ media: '' }, 'media', null), null);
  assert.deepEqual(Timeline.readJsonField({}, 'reactions', []), []);
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

test('row keeps the helper\'s own decode verdict for a video, and starts without one', () => {
  assert.strictEqual(Timeline.row({ id: 'a' }).mediaDecodeAs, '');
  assert.strictEqual(Timeline.row({ id: 'a', mediaDecodeAs: 'gif' }).mediaDecodeAs, 'gif');
});

test('row keeps the helper\'s own playable verdict for a video, and starts false', () => {
  assert.strictEqual(Timeline.row({ id: 'a' }).mediaPlayable, false);
  assert.strictEqual(Timeline.row({ id: 'a', mediaPlayable: true }).mediaPlayable, true);
});

test('row keeps the helper\'s own risky verdict, and starts not risky', () => {
  assert.strictEqual(Timeline.row({ id: 'a' }).mediaRisky, false);
  assert.strictEqual(Timeline.row({ id: 'a', mediaRisky: true }).mediaRisky, true);
});

test('row keeps whether a media fetch failed, and starts not failed', () => {
  assert.strictEqual(Timeline.row({ id: 'a' }).mediaFailed, false);
  assert.strictEqual(Timeline.row({ id: 'a', mediaFailed: true }).mediaFailed, true);
});

test('row keeps a failed fetch\'s safe reason category, and starts without one', () => {
  assert.strictEqual(Timeline.row({ id: 'a' }).mediaFailedReason, '');
  assert.strictEqual(Timeline.row({ id: 'a', mediaFailedReason: 'decrypt' }).mediaFailedReason, 'decrypt');
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

test('row keeps mentions as a string, so updating them later does not silently drop them', () => {
  const mentions = [{ userId: 'u1', name: 'Nadia', offset: 3, length: 6 }];

  assert.strictEqual(Timeline.row({ id: 'a' }).mentions, '[]');
  assert.strictEqual(Timeline.row({ id: 'a', mentions }).mentions, JSON.stringify(mentions));
});

test('row keeps mentionsMe as a plain boolean, defaulting to false', () => {
  assert.strictEqual(Timeline.row({ id: 'a' }).mentionsMe, false);
  assert.strictEqual(Timeline.row({ id: 'a', mentionsMe: true }).mentionsMe, true);
});

test('mentions reads a row\'s @-mention tokens back, or an empty list', () => {
  const mentions = [{ userId: 'u1', name: 'Nadia', offset: 3, length: 6 }];
  assert.deepEqual(Timeline.mentions({ mentions: JSON.stringify(mentions) }), mentions);
  assert.deepEqual(Timeline.mentions({ mentions: '' }), []);
  assert.deepEqual(Timeline.mentions({ mentions }), mentions);
  assert.deepEqual(Timeline.mentions({}), []);
});

test('row keeps formatting as a string, so updating it later does not silently drop it', () => {
  const formatting = [{ kind: 'link', start: 0, end: 4, url: 'https://x.io' }];

  assert.strictEqual(Timeline.row({ id: 'a' }).formatting, '[]');
  assert.strictEqual(Timeline.row({ id: 'a', formatting }).formatting, JSON.stringify(formatting));
});

test('formatting reads a row\'s rich-text spans back, or an empty list', () => {
  const formatting = [{ kind: 'bold', start: 0, end: 4 }];
  assert.deepEqual(Timeline.formatting({ formatting: JSON.stringify(formatting) }), formatting);
  assert.deepEqual(Timeline.formatting({ formatting: '' }), []);
  assert.deepEqual(Timeline.formatting({ formatting }), formatting);
  assert.deepEqual(Timeline.formatting({}), []);
});

test('row keeps retryAt, defaulting to 0 while no automatic retry is scheduled', () => {
  assert.strictEqual(Timeline.row({ id: 'a' }).retryAt, 0);
  assert.strictEqual(Timeline.row({ id: 'a', retryAt: 1234 }).retryAt, 1234);
});

test('mediaJustRetired is true the first time an outgoing photo reaches sent', () => {
  const oldRow = { status: 'pending' };
  const update = { outgoing: true, media: { kind: 'photo' }, status: 'sent' };
  assert.strictEqual(Timeline.mediaJustRetired(oldRow, update), true);
});

test('mediaJustRetired is true moving straight from pending to read', () => {
  const oldRow = { status: 'pending' };
  const update = { outgoing: true, media: { kind: 'file' }, status: 'read' };
  assert.strictEqual(Timeline.mediaJustRetired(oldRow, update), true);
});

test('mediaJustRetired is false once already confirmed, such as a later read receipt', () => {
  const oldRow = { status: 'sent' };
  const update = { outgoing: true, media: { kind: 'photo' }, status: 'delivered' };
  assert.strictEqual(Timeline.mediaJustRetired(oldRow, update), false);
});

test('mediaJustRetired is false for a message with no media', () => {
  const oldRow = { status: 'pending' };
  const update = { outgoing: true, status: 'sent' };
  assert.strictEqual(Timeline.mediaJustRetired(oldRow, update), false);
});

test('mediaJustRetired is false for an incoming message', () => {
  const oldRow = { status: 'received' };
  const update = { outgoing: false, media: { kind: 'photo' }, status: 'received' };
  assert.strictEqual(Timeline.mediaJustRetired(oldRow, update), false);
});

test('mediaJustRetired is false for an unrelated update, such as a reaction, that leaves status alone', () => {
  const oldRow = { status: 'sent' };
  const update = { outgoing: true, media: { kind: 'photo' }, status: 'sent' };
  assert.strictEqual(Timeline.mediaJustRetired(oldRow, update), false);
});

test('mediaIdentityChanged is true when an edit changes the media kind, such as a voice note edited into a photo', () => {
  const oldRow = { media: JSON.stringify({ kind: 'voice', fileName: 'note.ogg', duration: 5 }) };
  const update = { media: { kind: 'photo', fileName: 'note.ogg', width: 800, height: 600 } };
  assert.strictEqual(Timeline.mediaIdentityChanged(oldRow, update), true);
});

test('mediaIdentityChanged is true when the same kind points at different bytes', () => {
  const oldRow = { media: JSON.stringify({ kind: 'photo', fileName: 'a.jpg', width: 100, height: 100, size: 1000 }) };
  const update = { media: { kind: 'photo', fileName: 'b.jpg', width: 400, height: 400, size: 9000 } };
  assert.strictEqual(Timeline.mediaIdentityChanged(oldRow, update), true);
});

test('mediaIdentityChanged is false for a caption-only edit that leaves the same media alone', () => {
  const sameMedia = { kind: 'photo', fileName: 'a.jpg', width: 100, height: 100, size: 1000 };
  const oldRow = { media: JSON.stringify(sameMedia) };
  const update = { text: 'new caption', media: { ...sameMedia } };
  assert.strictEqual(Timeline.mediaIdentityChanged(oldRow, update), false);
});

test('mediaIdentityChanged is false for an unrelated update with no media at all', () => {
  const oldRow = { media: '' };
  const update = { text: 'hello' };
  assert.strictEqual(Timeline.mediaIdentityChanged(oldRow, update), false);
});

test('mediaIdentityChanged is true when media is added where there was none, or removed entirely', () => {
  const oldRow = { media: '' };
  assert.strictEqual(Timeline.mediaIdentityChanged(oldRow, { media: { kind: 'photo', fileName: 'a.jpg' } }), true);

  const withMedia = { media: JSON.stringify({ kind: 'photo', fileName: 'a.jpg' }) };
  assert.strictEqual(Timeline.mediaIdentityChanged(withMedia, { media: null }), true);
});

test('mediaIdentityChanged is false for a poll update that only changes tallies, never media.kind', () => {
  const oldRow = { media: JSON.stringify({ kind: 'poll', poll: { question: 'q', options: [] } }) };
  const update = { media: { kind: 'poll', poll: { question: 'q', options: [{ id: 'o1', votes: 3 }] } } };
  assert.strictEqual(Timeline.mediaIdentityChanged(oldRow, update), false);
});

// windowNewest: capping how many loaded rows the shell keeps.

test('windowNewest leaves a list at or under the cap alone', () => {
  const rows = [{ id: 'a' }, { id: 'b' }];
  assert.strictEqual(Timeline.windowNewest(rows, 2, ''), rows);
  assert.strictEqual(Timeline.windowNewest(rows, 5, ''), rows);
});

test('windowNewest drops the oldest rows, at the back of a newest-first list, past the cap', () => {
  const rows = [{ id: 'a' }, { id: 'b' }, { id: 'c' }, { id: 'd' }];
  const result = Timeline.windowNewest(rows, 2, '');
  assert.deepEqual(result.map((r) => r.id), ['a', 'b']);
});

test('windowNewest never drops the protected id, even past the cap', () => {
  const rows = [{ id: 'a' }, { id: 'b' }, { id: 'c' }, { id: 'd' }];
  const result = Timeline.windowNewest(rows, 2, 'd');
  assert.deepEqual(result.map((r) => r.id), ['a', 'b', 'c', 'd']);
});

test('windowNewest with no protected id loaded behaves as an ordinary cap', () => {
  const rows = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];
  const result = Timeline.windowNewest(rows, 2, 'not-loaded');
  assert.deepEqual(result.map((r) => r.id), ['a', 'b']);
});

// reindex: building an id -> position map from a newest-first list.

test('reindex maps every id to its position', () => {
  const order = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];
  assert.deepEqual(Timeline.reindex(order), { a: 0, b: 1, c: 2 });
});

test('reindex of an empty list is an empty map', () => {
  assert.deepEqual(Timeline.reindex([]), {});
});

// mergeRow: the media-field carry-over logic upsert applies to an
// existing row, pulled out of MessageTimeline.qml so it can be tested
// directly and reused for a flood of updates.

test('mergeRow keeps a resolved mediaPath and verdicts across an unrelated update', () => {
  const oldRow = { status: 'sent', mediaPath: '/m/a.jpg', mediaPlayable: true, mediaVerdictsKnown: true };
  const merged = Timeline.mergeRow(oldRow, { id: 'a', status: 'sent', text: 'edited' });
  assert.strictEqual(merged.mediaPath, '/m/a.jpg');
  assert.strictEqual(merged.mediaPlayable, true);
  assert.strictEqual(merged.text, 'edited');
});

test('mergeRow drops a stale mediaPath the moment an outgoing photo is first confirmed sent', () => {
  const oldRow = { status: 'pending', mediaPath: '/m/outgoing/a.jpg' };
  const merged = Timeline.mergeRow(oldRow, { outgoing: true, media: { kind: 'photo' }, status: 'sent' });
  assert.strictEqual(merged.mediaPath, '');
});

// applyUpsert and applyRemove: the decisions MessageTimeline.upsert and
// .remove apply to the model - placement, the id index and which rows
// an annotation must be recomputed for - kept pure so a flood can be
// proven fast without any QML involved.

test('applyUpsert updates an existing row in place and marks it and its newer neighbour touched', () => {
  let order = [{ id: 'b', created: 20 }, { id: 'a', created: 10 }];
  let index = Timeline.reindex(order);

  const result = Timeline.applyUpsert(order, index, { id: 'a', created: 10, text: 'edited' }, 100, '');
  assert.strictEqual(result.update, true);
  assert.strictEqual(result.at, 1);
  assert.deepEqual(result.touched.slice().sort(), [0, 1]);
  assert.strictEqual(result.evicted, 0);
});

test('applyUpsert inserts a brand new message by time and marks the insertion point and its newer neighbour touched', () => {
  const order = [{ id: 'b', created: 20 }, { id: 'a', created: 10 }];
  const index = Timeline.reindex(order);

  const result = Timeline.applyUpsert(order, index, { id: 'c', created: 30 }, 100, '');
  assert.strictEqual(result.update, false);
  assert.strictEqual(result.at, 0);
  assert.deepEqual(result.order.map((r) => r.id), ['c', 'b', 'a']);
  assert.deepEqual(result.index, { c: 0, b: 1, a: 2 });
  assert.deepEqual(result.touched, [0]);
  assert.strictEqual(result.evicted, 0);
});

test('applyUpsert evicts the oldest row once a new one pushes past the cap, and marks the new oldest touched too', () => {
  const order = [{ id: 'b', created: 20 }, { id: 'a', created: 10 }];
  const index = Timeline.reindex(order);

  const result = Timeline.applyUpsert(order, index, { id: 'c', created: 30 }, 2, '');
  assert.strictEqual(result.evicted, 1);
  assert.deepEqual(result.order.map((r) => r.id), ['c', 'b']);
  assert.deepEqual(result.index, { c: 0, b: 1 });
  assert.ok(result.touched.includes(1), 'expected the new oldest row to be marked touched: ' + JSON.stringify(result.touched));
});

test('applyUpsert never evicts the protected id even once the cap is reached', () => {
  const order = [{ id: 'b', created: 20 }, { id: 'a', created: 10 }];
  const index = Timeline.reindex(order);

  const result = Timeline.applyUpsert(order, index, { id: 'c', created: 30 }, 2, 'a');
  assert.strictEqual(result.evicted, 0);
  assert.deepEqual(result.order.map((r) => r.id), ['c', 'b', 'a']);
});

test('applyRemove drops a loaded row and marks its newer neighbour touched', () => {
  const order = [{ id: 'c', created: 30 }, { id: 'b', created: 20 }, { id: 'a', created: 10 }];
  const index = Timeline.reindex(order);

  const result = Timeline.applyRemove(order, index, 'b');
  assert.strictEqual(result.removed, true);
  assert.strictEqual(result.at, 1);
  assert.deepEqual(result.order.map((r) => r.id), ['c', 'a']);
  assert.deepEqual(result.touched, [0]);
});

test('applyRemove does nothing for an id that is not loaded', () => {
  const order = [{ id: 'a', created: 10 }];
  const index = Timeline.reindex(order);

  const result = Timeline.applyRemove(order, index, 'missing');
  assert.strictEqual(result.removed, false);
  assert.strictEqual(result.order, order);
  assert.deepEqual(result.touched, []);
});

test('applyRemove of the only loaded row touches nothing, since it had no newer neighbour', () => {
  const order = [{ id: 'a', created: 10 }];
  const index = Timeline.reindex(order);

  const result = Timeline.applyRemove(order, index, 'a');
  assert.deepEqual(result.order, []);
  assert.deepEqual(result.touched, []);
});

// annotateForPair: the per-row half of annotate(), so a caller that
// only touched one or two rows need not walk every loaded message.

test('annotateForPair matches annotateAt for every row of a mixed list', () => {
  const msgs = [
    { id: 'm3', created: new Date(2026, 9, 6, 14, 0).getTime(), senderId: 'alice', outgoing: false },
    { id: 'm2', created: new Date(2026, 9, 6, 13, 55).getTime(), senderId: 'alice', outgoing: false },
    { id: 'm1', created: new Date(2026, 9, 5, 10, 0).getTime(), senderId: 'bob', outgoing: true }
  ];
  for (let i = 0; i < msgs.length; i++) {
    const viaPair = Timeline.annotateForPair(msgs[i], msgs[i + 1] ?? null, true, nowMs);
    const viaAt = Timeline.annotateAt(msgs, i, true, nowMs);
    assert.deepEqual(viaPair, viaAt, `mismatch at index ${i}`);
  }
});

// A flood of live messages must cost roughly the same per message
// however many have already arrived: MAX_LOADED_ROWS keeps the index
// rebuild applyUpsert does on every insert bounded by a small constant,
// not by how many of the 50,000 have already landed.

test('applyUpsert handles a flood of 50000 new messages within a generous time bound', () => {
  let order = [];
  let index = {};
  const start = Date.now();

  for (let i = 0; i < 50000; i++) {
    const result = Timeline.applyUpsert(order, index, { id: `m${i}`, created: i }, Timeline.MAX_LOADED_ROWS, '');
    order = result.order;
    index = result.index;
  }

  const elapsedMs = Date.now() - start;
  assert.ok(elapsedMs < 8000, `50000 upserts took ${elapsedMs}ms, want well under 8000ms`);
  assert.strictEqual(order.length, Timeline.MAX_LOADED_ROWS);
  assert.strictEqual(order[0].id, 'm49999');
  assert.strictEqual(order[order.length - 1].id, `m${50000 - Timeline.MAX_LOADED_ROWS}`);
  assert.strictEqual(Object.keys(index).length, Timeline.MAX_LOADED_ROWS);
  assert.strictEqual(index['m49999'], 0);
});
