'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Palette = load('lib/Palette.js');
const Format = load('lib/Format.js');

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
    { id: 'a', title: 'Alpha', titleVerified: true, service: 'whatsapp', match: 'see you then', matchMessageId: 'm1', matchSender: 'Alex', matchSenderVerified: true },
    { id: 'b', title: 'Beta', service: 'telegram' }, // title-only match: no matched message
    { id: 'c', title: 'Gamma', titleVerified: false, service: 'whatsapp', match: 'ok', matchMessageId: 'm3', matchSender: 'Mum', matchSenderVerified: false }
  ];

  assert.deepEqual(Palette.messageRows(conversations), [
    { conversationId: 'a', messageId: 'm1', sender: 'Alex', senderVerified: true, conversationTitle: 'Alpha', conversationTitleVerified: true, service: 'whatsapp', snippet: 'see you then' },
    { conversationId: 'c', messageId: 'm3', sender: 'Mum', senderVerified: false, conversationTitle: 'Gamma', conversationTitleVerified: false, service: 'whatsapp', snippet: 'ok' }
  ]);
});

test('messageRows: handles null or undefined input', () => {
  assert.deepEqual(Palette.messageRows(null), []);
  assert.deepEqual(Palette.messageRows(undefined), []);
});

// messageRows: a conversation with no matchSenderVerified or
// titleVerified of its own - an older helper, or the self-chat, which
// carries no sender at all - falls back to unverified, the same
// fail-safe default every other surface gives a missing flag (see
// Verify.markedName and docs/decisions.md).
test('messageRows: falls back to an empty, unverified sender or snippet', () => {
  const conversations = [{ id: 'a', title: 'Alpha', matchMessageId: 'm1' }];

  assert.deepEqual(Palette.messageRows(conversations), [
    { conversationId: 'a', messageId: 'm1', sender: '', senderVerified: false, conversationTitle: 'Alpha', conversationTitleVerified: false, service: undefined, snippet: '' }
  ]);
});

// messageRows: caps an oversized snippet, as defence in depth behind
// the helper's own bounded snippet() window, the same cap
// Format.previewText already gives a conversation row's own preview.
test('messageRows: caps an oversized snippet to MAX_PREVIEW_LENGTH', () => {
  const huge = 'x'.repeat(500);
  const conversations = [{ id: 'a', title: 'Alpha', matchMessageId: 'm1', match: huge }];

  const got = Palette.messageRows(conversations);
  assert.equal(got[0].snippet.length, Format.MAX_PREVIEW_LENGTH);
  assert.equal(got[0].snippet, huge.slice(0, Format.MAX_PREVIEW_LENGTH));
});

test('results/items: "conversations" mode matches the list and appends the "Messages" section', () => {
  const conversations = [
    { id: 'a', title: 'Alex Chen', titleVerified: true, service: 'whatsapp', unread: 2, lastActivity: 200 },
    { id: 'b', title: 'Mum', titleVerified: true, service: 'telegram', unread: 0, lastActivity: 100 }
  ];
  const ctx = {
    query: 'al',
    listController: { all: conversations },
    service: { services: [] },
    messageResults: Palette.messageRows([{ id: 'c', title: 'Group', titleVerified: true, matchMessageId: 'm1', matchSender: 'Sam', matchSenderVerified: true, match: 'see you', service: 'telegram' }])
  };

  const results = Palette.results('conversations', ctx);
  assert.deepEqual(results.map((c) => c.id), ['a']);

  const items = Palette.items('conversations', results, ctx);
  assert.deepEqual(items, [
    { label: 'Alex Chen', detail: 'WhatsApp', keys: '', unread: 2, section: 'conversation' },
    { label: 'Sam: see you', detail: 'Group', keys: '', unread: 0, section: 'message' }
  ]);
});

// results/items: a self-chosen conversation title or matched-message
// sender that is not verified must still read as "~Mum", everywhere
// this mode shows one, so a stranger naming a chat or themselves
// "Mum" never looks like a saved contact in the unified palette either
// (see docs/decisions.md).
test('results/items: "conversations" mode marks an unverified title and matched sender', () => {
  const conversations = [{ id: 'a', title: 'Mum', titleVerified: false, service: 'whatsapp', unread: 0, lastActivity: 100 }];
  const ctx = {
    query: '',
    listController: { all: conversations },
    service: { services: [] },
    messageResults: Palette.messageRows([
      { id: 'b', title: 'Mum', titleVerified: false, matchMessageId: 'm1', matchSender: 'Mum', matchSenderVerified: false, match: 'hello', service: 'whatsapp' }
    ])
  };

  const items = Palette.items('conversations', Palette.results('conversations', ctx), ctx);

  assert.deepEqual(items, [
    { label: '~Mum', detail: 'WhatsApp', keys: '', unread: 0, section: 'conversation' },
    { label: '~Mum: hello', detail: '~Mum', keys: '', unread: 0, section: 'message' }
  ]);
});

test('results/items: "links" mode matches and labels the offered urls, passing displayText through unchanged', () => {
  const ctx = { query: 'exam', linkChoices: [
    { url: 'https://example.com', displayText: 'https://example.com' },
    { url: 'https://other.test', displayText: 'https://other.test' }
  ] };

  const results = Palette.results('links', ctx);
  assert.deepEqual(results, [{ url: 'https://example.com', displayText: 'https://example.com' }]);
  assert.deepEqual(Palette.items('links', results, ctx), [{ label: 'Open link: https://example.com', detail: '', keys: '' }]);
});

test('results/items: "snoozeCustom" mode previews the typed time', () => {
  const ctx = { query: '2h', now: Date.parse('2026-01-01T10:00:00Z') };

  const results = Palette.results('snoozeCustom', ctx);
  assert.strictEqual(results.length, 1);
  assert.deepEqual(Palette.items('snoozeCustom', results, ctx), [{ label: results[0].label, detail: '', keys: '' }]);
});

test('results/items: "sendLaterCustom" mode previews the typed time as a send time', () => {
  const ctx = { query: '2h', now: Date.parse('2026-01-01T10:00:00Z') };

  const results = Palette.results('sendLaterCustom', ctx);
  assert.deepEqual(results, [{ at: Date.parse('2026-01-01T12:00:00Z') }]);
  assert.deepEqual(Palette.items('sendLaterCustom', results, ctx), [{ label: 'Sends at 12:00', detail: '', keys: '' }]);
});

test('results/items: "sendLaterCustom" mode has nothing to show for unparsed text', () => {
  const ctx = { query: 'not a time', now: Date.parse('2026-01-01T10:00:00Z') };

  assert.deepEqual(Palette.results('sendLaterCustom', ctx), []);
});

test('results/items: "sendLaterCustom" mode lists the named presets before anything is typed', () => {
  const ctx = { query: '', now: Date.parse('2026-01-01T10:00:00Z') };

  const results = Palette.results('sendLaterCustom', ctx);
  assert.deepEqual(results.map((r) => r.label), ['In 5 minutes', 'In 30 minutes', 'In 1 hour', 'Tonight', 'Tomorrow morning', 'Monday morning']);

  const items = Palette.items('sendLaterCustom', results, ctx);
  assert.deepEqual(items[0], { label: 'In 5 minutes', detail: '10:05', keys: '' });
});

test('results/items: "sendLaterCustom" mode narrows the presets by name', () => {
  const ctx = { query: 'tomorrow', now: Date.parse('2026-01-01T10:00:00Z') };

  const results = Palette.results('sendLaterCustom', ctx);
  assert.deepEqual(results.map((r) => r.label), ['Tomorrow morning']);
});

test('results/items: "scheduledMessages" mode lists queued messages, matched against their text', () => {
  const now = Date.parse('2026-01-01T10:00:00Z');
  const ctx = {
    query: '',
    now,
    scheduledMessages: [
      { id: 's1', conversationId: 'a', text: 'see you soon', sendAt: Date.parse('2026-01-01T18:00:00Z') },
      { id: 's2', conversationId: 'b', text: 'happy birthday', sendAt: Date.parse('2026-01-02T09:00:00Z') }
    ]
  };

  const results = Palette.results('scheduledMessages', ctx);
  assert.deepEqual(results.map((m) => m.id), ['s1', 's2']);

  const items = Palette.items('scheduledMessages', results, ctx);
  assert.deepEqual(items, [
    { label: 'Sends at 18:00: see you soon', detail: '', keys: '', conversationId: 'a', id: 's1', text: 'see you soon' },
    { label: 'Sends at Tomorrow 09:00: happy birthday', detail: '', keys: '', conversationId: 'b', id: 's2', text: 'happy birthday' }
  ]);
});

test('results/items: "scheduledMessages" mode narrows by the typed query', () => {
  const now = Date.parse('2026-01-01T10:00:00Z');
  const ctx = {
    query: 'birthday',
    now,
    scheduledMessages: [
      { id: 's1', conversationId: 'a', text: 'see you soon', sendAt: Date.parse('2026-01-01T18:00:00Z') },
      { id: 's2', conversationId: 'b', text: 'happy birthday', sendAt: Date.parse('2026-01-02T09:00:00Z') }
    ]
  };

  const results = Palette.results('scheduledMessages', ctx);
  assert.deepEqual(results.map((m) => m.id), ['s2']);
});

test('results/items: "scheduledMessagesForChat" mode narrows to the open conversation', () => {
  const now = Date.parse('2026-01-01T10:00:00Z');
  const ctx = {
    query: '',
    now,
    conversationId: 'a',
    scheduledMessages: [
      { id: 's1', conversationId: 'a', text: 'see you soon', sendAt: Date.parse('2026-01-01T18:00:00Z') },
      { id: 's2', conversationId: 'b', text: 'happy birthday', sendAt: Date.parse('2026-01-02T09:00:00Z') },
      { id: 's3', conversationId: 'a', text: 'call you later', sendAt: Date.parse('2026-01-03T09:00:00Z') }
    ]
  };

  const results = Palette.results('scheduledMessagesForChat', ctx);
  assert.deepEqual(results.map((m) => m.id), ['s1', 's3']);

  const items = Palette.items('scheduledMessagesForChat', results, ctx);
  assert.deepEqual(items.map((i) => i.id), ['s1', 's3']);
});

test('results/items: "keyBindings" mode matches the bindings report, shown as-is', () => {
  const bindings = [{ action: 'chat.open', keys: ['Enter'], contexts: ['list'], label: 'Open chat', overridden: false }];
  const ctx = { query: 'open', bindings, keyBindingConflicts: [], keyBindingErrors: [] };

  const results = Palette.results('keyBindings', ctx);
  assert.deepEqual(results.map((r) => r.label), ['Open chat']);
  assert.strictEqual(Palette.items('keyBindings', results, ctx), results);
});

test('results/items: the default "commands" mode matches the command palette list, shown as-is', () => {
  const ctx = { query: 'quit', bindings: [{ action: 'app.quit', keys: ['Ctrl+Q'], contexts: ['global'], label: 'Quit OmaMessenger', command: true }] };

  const results = Palette.results('commands', ctx);
  assert.deepEqual(results, [{ action: 'app.quit', label: 'Quit OmaMessenger', keys: 'Ctrl+Q', keywords: [], scoped: false }]);
  assert.strictEqual(Palette.items('commands', results, ctx), results);
});

// An exact word or prefix match must always outrank a scattered fuzzy
// match, no matter how long the fuzzy candidate is: a short real command
// must never lose to a long label that merely happens to contain the
// same letters somewhere inside it, far apart.
test('score: a word-start match always outranks a scattered fuzzy match, regardless of label length', () => {
  const exact = 'Pin or unpin chat';
  const scattered = 'Please if nobody intervenes, we negotiate near opportunities involving nothing';

  const ranked = Palette.search([scattered, exact], 'pin', (s) => s);

  assert.deepEqual(ranked, [exact, scattered]);
});

// A tie must never be won by a compound action (one that does more than
// the query asked for) over a plain one that matches exactly as well:
// see chat.archiveRead ("Archive and mark read"), the command issue #33
// reported winning queries meant for a plain "mark unread" lookup.
test('commands mode: a tie is never won by a compound action like chat.archiveRead', () => {
  const bindings = [
    { action: 'chat.archiveRead', keys: [], contexts: ['global'], label: 'Mark read', command: true },
    { action: 'chat.markUnread', keys: [], contexts: ['global'], label: 'Mark read', command: true }
  ];
  const ctx = { query: 'mark', bindings };

  const results = Palette.results('commands', ctx);
  const items = Palette.items('commands', results, ctx);

  assert.strictEqual(items[0].action, 'chat.markUnread');
});

// The command palette's real list, end to end: typing "unread" or "mark
// un" looking for a way to flag a chat unread again must never surface
// the compound "Archive and mark read" command, which does something the
// query never asked for, and once "Mark as unread" exists, "mark un"
// uniquely names it.
test('commands mode: real queries for "unread" never surface "Archive and mark read" first', () => {
  const cases = ['unread', 'mark un'];

  for (const query of cases) {
    const results = Palette.results('commands', { query });
    const items = Palette.items('commands', results, { query });

    assert.notStrictEqual(items[0] && items[0].label, 'Archive and mark read', `query "${query}"`);
  }

  const results = Palette.results('commands', { query: 'mark un' });
  const items = Palette.items('commands', results, { query: 'mark un' });
  assert.strictEqual(items[0].label, 'Mark as unread');
});

// search: a command may declare extra search keywords (an array from
// textOf, rather than a single string), and the best-scoring one of
// them counts, so a label that only matches a query by a scattered
// fuzzy subsequence still ranks by an exact keyword instead.
test('search: textOf may return several candidate texts per item, and the best one wins', () => {
  const items = [
    { label: 'Mark as unread', keywords: ['read'] },
    { label: 'React to the highlighted message', keywords: [] }
  ];

  const ranked = Palette.search(items, 'read', (c) => [c.label].concat(c.keywords));

  assert.strictEqual(ranked[0].label, 'Mark as unread');
});

// A tie between a command scoped to the chat someone has open or
// highlighted and a global setting or toggle must go to the scoped one:
// the query is almost always about that chat, not a global toggle that
// only happens to score the same.
test('commands mode: a tie prefers a command scoped to the current chat over a global one', () => {
  const bindings = [
    { action: 'settings.toggleReadReceipts', keys: [], contexts: ['global'], label: 'Toggle read receipts', command: true },
    { action: 'chat.markUnread', keys: [], contexts: ['list', 'conversation'], label: 'Mark unread', command: true, keywords: ['read'] }
  ];

  const results = Palette.results('commands', { query: 'read', bindings });

  assert.strictEqual(results[0].action, 'chat.markUnread');
});

// The real command list, end to end, for the query the user actually
// typed looking for a way to flag a chat unread again: "read". The
// chat's own read-state commands ("Mark as unread", "Mark as read")
// must come first, ahead of a global toggle that only happens to
// contain the same word ("Toggle read receipts"), which in turn comes
// ahead of compound actions that do more than the query asked for
// ("Archive and mark read", "Archive all read conversations"), which in
// turn come ahead of anything that merely contains "read" scattered
// inside another word or elsewhere in a longer label ("React to the
// highlighted message", "Show/Next/Previous unread conversation(s)").
test('commands mode: real "read" query ranks the chat\'s own read-state commands first, then a global toggle, then compound actions, then scattered matches', () => {
  const results = Palette.results('commands', { query: 'read' });
  const actions = results.map((c) => c.action);

  assert.deepEqual(actions.slice(0, 2).sort(), ['chat.markRead', 'chat.markUnread']);

  const toggle = actions.indexOf('settings.toggleReadReceipts');
  const archiveRead = actions.indexOf('chat.archiveRead');
  const archiveAllRead = actions.indexOf('list.archiveAllRead');
  const react = actions.indexOf('message.react');

  assert.strictEqual(toggle, 2);
  assert.ok(archiveRead > toggle, 'compound chat.archiveRead must rank below the global toggle');
  assert.ok(archiveAllRead > archiveRead, 'list.archiveAllRead must rank below chat.archiveRead');
  assert.ok(react > archiveAllRead, 'a scattered match like message.react must rank below every exact-word match');
});

// The report this covers found "Scheduled messages" unreachable by the
// one other word someone would type for it ("send later", the
// composer's own button label): its own label never says that, so it
// needs the keyword the same way "Mark as unread" needs "read".
test('commands mode: the real command list finds "Scheduled messages" by "scheduled" or "send later"', () => {
  const byScheduled = Palette.results('commands', { query: 'scheduled' }).map((c) => c.action);
  assert.ok(byScheduled.includes('scheduled.show'));

  const bySendLater = Palette.results('commands', { query: 'send later' }).map((c) => c.action);
  assert.ok(bySendLater.includes('scheduled.show'));
});

// The real command list for "unread": "Mark as unread" must come first,
// ahead of the global unread-navigation commands that also contain the
// literal word "unread" ("Show/Next/Previous unread conversation(s)"),
// and the compound "Archive and mark read"/"Archive all read
// conversations" must never appear at all, since neither contains the
// letter "u".
test('commands mode: real "unread" query puts "Mark as unread" first and never surfaces a compound action', () => {
  const results = Palette.results('commands', { query: 'unread' });
  const actions = results.map((c) => c.action);

  assert.strictEqual(actions[0], 'chat.markUnread');
  assert.ok(!actions.includes('chat.archiveRead'));
  assert.ok(!actions.includes('list.archiveAllRead'));
});

// The real command list for "mark": both read-state commands, the only
// two whose label starts with the word "mark", rank first, in either
// order.
test('commands mode: real "mark" query surfaces both read-state commands first', () => {
  const results = Palette.results('commands', { query: 'mark' });
  const top = results.slice(0, 2).map((c) => c.action).sort();

  assert.deepEqual(top, ['chat.markRead', 'chat.markUnread']);
});

// The real command list for "archive": the plain "Archive or unarchive
// chat" must rank above the two compound actions that also mark the
// chat read, chat.archiveRead before list.archiveAllRead (their
// original order, since both are compound and otherwise tied).
test('commands mode: real "archive" query ranks the plain archive command above the compound ones', () => {
  const results = Palette.results('commands', { query: 'archive' });
  const actions = results.map((c) => c.action);

  assert.strictEqual(actions[0], 'chat.archive');
  const archiveRead = actions.indexOf('chat.archiveRead');
  const archiveAllRead = actions.indexOf('list.archiveAllRead');
  assert.ok(archiveRead > 0);
  assert.ok(archiveAllRead > archiveRead);
});
