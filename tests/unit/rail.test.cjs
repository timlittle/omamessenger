'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Rail = load('lib/Rail.js');

test('items: orders entries as all, then services in order whatsapp/telegram', () => {
  const accounts = [
    { id: 'a1', service: 'telegram', name: 'Tel1', status: 'connected' },
    { id: 'a2', service: 'whatsapp', name: 'WA1', status: 'connected' }
  ];
  const conversations = [];

  const result = Rail.items(accounts, conversations);

  assert.strictEqual(result[0].kind, 'all');
  assert.strictEqual(result[1].kind, 'service');
  assert.strictEqual(result[1].service, 'whatsapp');
  assert.strictEqual(result[2].kind, 'service');
  assert.strictEqual(result[2].service, 'telegram');
});

test('items: includes account entries only when service has more than one', () => {
  const accounts = [
    { id: 'a1', service: 'whatsapp', name: 'WA1', status: 'connected' },
    { id: 'a2', service: 'whatsapp', name: 'WA2', status: 'connected' },
    { id: 'a3', service: 'telegram', name: 'Tel1', status: 'connected' }
  ];
  const conversations = [];

  const result = Rail.items(accounts, conversations);

  // all, service:whatsapp, account:a1, account:a2, service:telegram
  assert.strictEqual(result.length, 5);
  assert.strictEqual(result[0].kind, 'all');
  assert.strictEqual(result[1].kind, 'service');
  assert.strictEqual(result[2].kind, 'account');
  assert.strictEqual(result[3].kind, 'account');
  assert.strictEqual(result[4].kind, 'service');
});

test('items: sorts accounts alphabetically within a service', () => {
  const accounts = [
    { id: 'a1', service: 'whatsapp', name: 'Zebra', status: 'connected' },
    { id: 'a2', service: 'whatsapp', name: 'Apple', status: 'connected' },
    { id: 'a3', service: 'whatsapp', name: 'Mango', status: 'connected' }
  ];
  const conversations = [];

  const result = Rail.items(accounts, conversations);

  assert.strictEqual(result[2].label, 'Apple');
  assert.strictEqual(result[3].label, 'Mango');
  assert.strictEqual(result[4].label, 'Zebra');
});

test('items: skips services with no accounts', () => {
  const accounts = [
    { id: 'a1', service: 'whatsapp', name: 'WA1', status: 'connected' }
  ];
  const conversations = [];

  const result = Rail.items(accounts, conversations);

  // all, service:whatsapp
  assert.strictEqual(result.length, 2);
  assert.strictEqual(result[1].service, 'whatsapp');
});

test('items: unread sums matching conversations excluding muted', () => {
  const accounts = [
    { id: 'a1', service: 'whatsapp', name: 'WA1', status: 'connected' }
  ];
  const conversations = [
    { id: 'c1', accountId: 'a1', service: 'whatsapp', unread: 3, muted: false },
    { id: 'c2', accountId: 'a1', service: 'whatsapp', unread: 2, muted: false },
    { id: 'c3', accountId: 'a1', service: 'whatsapp', unread: 5, muted: true }
  ];

  const result = Rail.items(accounts, conversations);

  assert.strictEqual(result[0].unread, 5); // all: 3 + 2
  assert.strictEqual(result[1].unread, 5); // service: 3 + 2
});

test('items: unread is zero when no matching conversations', () => {
  const accounts = [
    { id: 'a1', service: 'whatsapp', name: 'WA1', status: 'connected' },
    { id: 'a2', service: 'telegram', name: 'Tel1', status: 'connected' }
  ];
  const conversations = [
    { id: 'c1', accountId: 'a1', service: 'whatsapp', unread: 3, muted: false }
  ];

  const result = Rail.items(accounts, conversations);

  const telService = result.find((e) => e.key === 'service:telegram');
  assert.strictEqual(telService.unread, 0);
});

test('items: status ranks error > connecting > offline > connected', () => {
  const cases = [
    [['connected'], 'connected'],
    [['error'], 'error'],
    [['connected', 'error'], 'error'],
    [['connected', 'connecting', 'offline'], 'connecting'],
    [['connecting', 'offline'], 'connecting'],
    [['offline', 'connected'], 'offline'],
    [['error', 'connecting'], 'error']
  ];

  for (const [statuses, want] of cases) {
    const accounts = statuses.map((s, i) => ({
      id: `a${i}`,
      service: 'whatsapp',
      name: `WA${i}`,
      status: s
    }));

    const result = Rail.items(accounts, []);

    assert.strictEqual(result[0].status, want, `statuses ${statuses.join(',')} should give ${want}`);
  }
});

test('items: empty account list returns only all entry with connected status', () => {
  const result = Rail.items([], []);

  assert.strictEqual(result.length, 1);
  assert.strictEqual(result[0].key, 'all');
  assert.strictEqual(result[0].status, 'connected');
});

test('items: labels a service from knownServices when given', () => {
  const accounts = [{ id: 'a1', service: 'telegram', name: 'Tel1', status: 'connected' }];
  const known = [{ id: 'telegram', name: 'Telegram (beta)' }];

  const result = Rail.items(accounts, [], known);

  assert.strictEqual(result[1].label, 'Telegram (beta)');
});

test('items: handles null or undefined inputs', () => {
  assert.deepEqual(Rail.items(null, []), []);
  assert.deepEqual(Rail.items([], null), []);
  assert.deepEqual(Rail.items(undefined, []), []);
});

test('filter: returns all conversations for "all" key', () => {
  const conversations = [
    { id: 'c1', accountId: 'a1' },
    { id: 'c2', accountId: 'a2' }
  ];

  const result = Rail.filter(conversations, 'all');

  assert.strictEqual(result.length, 2);
  assert.deepEqual(result, conversations);
});

test('filter: returns service conversations for "service:<s>" key', () => {
  const conversations = [
    { id: 'c1', service: 'whatsapp', accountId: 'a1' },
    { id: 'c2', service: 'whatsapp', accountId: 'a2' },
    { id: 'c3', service: 'telegram', accountId: 'a3' }
  ];

  const result = Rail.filter(conversations, 'service:whatsapp');

  assert.strictEqual(result.length, 2);
  assert.strictEqual(result[0].id, 'c1');
  assert.strictEqual(result[1].id, 'c2');
});

test('filter: returns account conversations for "account:<id>" key', () => {
  const conversations = [
    { id: 'c1', accountId: 'a1' },
    { id: 'c2', accountId: 'a1' },
    { id: 'c3', accountId: 'a2' }
  ];

  const result = Rail.filter(conversations, 'account:a1');

  assert.strictEqual(result.length, 2);
  assert.strictEqual(result[0].id, 'c1');
  assert.strictEqual(result[1].id, 'c2');
});

test('filter: preserves order of conversations', () => {
  const conversations = [
    { id: 'c1', accountId: 'a1' },
    { id: 'c2', accountId: 'a2' },
    { id: 'c3', accountId: 'a1' }
  ];

  const result = Rail.filter(conversations, 'account:a1');

  assert.strictEqual(result.length, 2);
  assert.strictEqual(result[0].id, 'c1');
  assert.strictEqual(result[1].id, 'c3');
});

test('filter: returns all conversations for unknown key', () => {
  const conversations = [
    { id: 'c1', accountId: 'a1' },
    { id: 'c2', accountId: 'a2' }
  ];

  const result = Rail.filter(conversations, 'unknown:key');

  assert.strictEqual(result.length, 2);
});

test('filter: returns empty array for null/undefined inputs', () => {
  assert.deepEqual(Rail.filter(null, 'all'), []);
  assert.deepEqual(Rail.filter([], null), []);
  assert.deepEqual(Rail.filter(undefined, 'all'), []);
});

test('next: moves forward by delta', () => {
  const items = [
    { key: 'all' },
    { key: 'service:whatsapp' },
    { key: 'service:telegram' }
  ];

  assert.strictEqual(Rail.next(items, 'all', 1), 'service:whatsapp');
  assert.strictEqual(Rail.next(items, 'service:whatsapp', 1), 'service:telegram');
});

test('next: wraps around going forward', () => {
  const items = [
    { key: 'all' },
    { key: 'service:whatsapp' },
    { key: 'service:telegram' }
  ];

  assert.strictEqual(Rail.next(items, 'service:telegram', 1), 'all');
  assert.strictEqual(Rail.next(items, 'service:telegram', 2), 'service:whatsapp');
});

test('next: moves backward by negative delta', () => {
  const items = [
    { key: 'all' },
    { key: 'service:whatsapp' },
    { key: 'service:telegram' }
  ];

  assert.strictEqual(Rail.next(items, 'service:telegram', -1), 'service:whatsapp');
  assert.strictEqual(Rail.next(items, 'all', -1), 'service:telegram');
});

test('next: wraps around going backward', () => {
  const items = [
    { key: 'all' },
    { key: 'service:whatsapp' }
  ];

  assert.strictEqual(Rail.next(items, 'all', -1), 'service:whatsapp');
  assert.strictEqual(Rail.next(items, 'all', -2), 'all');
});

test('next: defaults to first item for unknown key', () => {
  const items = [
    { key: 'all' },
    { key: 'service:whatsapp' }
  ];

  assert.strictEqual(Rail.next(items, 'unknown', 0), 'all');
  assert.strictEqual(Rail.next(items, 'unknown', 1), 'service:whatsapp');
});

test('next: returns "all" for empty items', () => {
  assert.strictEqual(Rail.next([], 'any', 1), 'all');
});

test('next: handles null or undefined items', () => {
  assert.strictEqual(Rail.next(null, 'any', 1), 'all');
  assert.strictEqual(Rail.next(undefined, 'any', 1), 'all');
});

test('serviceLabel names each service for display', () => {
  assert.strictEqual(Rail.serviceLabel('whatsapp'), 'WhatsApp');
  assert.strictEqual(Rail.serviceLabel('telegram'), 'Telegram');
  assert.strictEqual(Rail.serviceLabel('signal'), 'signal');
});

test('serviceLabel prefers the helper\'s own name when it names the service', () => {
  const known = [{ id: 'telegram', name: 'Telegram (beta)' }];

  assert.strictEqual(Rail.serviceLabel('telegram', known), 'Telegram (beta)');
  assert.strictEqual(Rail.serviceLabel('whatsapp', known), 'WhatsApp');
  assert.strictEqual(Rail.serviceLabel('telegram', []), 'Telegram');
  assert.strictEqual(Rail.serviceLabel('telegram', null), 'Telegram');
});

test('accountLabel says which service an account belongs to', () => {
  assert.strictEqual(Rail.accountLabel({ service: 'telegram', name: 'Work' }), 'Telegram · Work');
});

test('accountLabel uses the helper\'s own service name when given', () => {
  const known = [{ id: 'telegram', name: 'Telegram (beta)' }];

  assert.strictEqual(Rail.accountLabel({ service: 'telegram', name: 'Work' }, known), 'Telegram (beta) · Work');
});

test('items: an account waiting for sign-in ranks worst', () => {
  const accounts = [
    { id: 'a1', service: 'telegram', name: 'T1', status: 'error' },
    { id: 'a2', service: 'telegram', name: 'T2', status: 'needs-auth' }
  ];

  assert.strictEqual(Rail.items(accounts, [])[0].status, 'needs-auth');
});

test('statusLabel names each status in words', () => {
  assert.strictEqual(Rail.statusLabel('needs-auth'), 'Sign-in needed');
  assert.strictEqual(Rail.statusLabel('connecting'), 'Connecting');
  assert.strictEqual(Rail.statusLabel(''), '');
});

test('accountNames maps each account id to its name', () => {
  const accounts = [{ id: 'a1', name: 'Personal' }, { id: 'a2', name: 'Work' }];

  assert.deepEqual(Rail.accountNames(accounts), { a1: 'Personal', a2: 'Work' });
  assert.deepEqual(Rail.accountNames(null), {});
});

test('multiAccountServices lists the services the rail split by account', () => {
  const items = [
    { kind: 'all' }, { kind: 'service', service: 'telegram' },
    { kind: 'account', service: 'telegram' }, { kind: 'account', service: 'telegram' }
  ];

  assert.deepEqual(Rail.multiAccountServices(items), ['telegram']);
});

test('accountDescription names an account by who is signed in when known', () => {
  assert.strictEqual(Rail.accountDescription({ service: 'telegram', name: 'Telegram', detail: 'Signed in as Tim' }), 'Telegram · Signed in as Tim');
  assert.strictEqual(Rail.accountDescription({ service: 'telegram', name: 'Work', detail: '' }), 'Telegram · Work');
});

test('accountDescription uses the helper\'s own service name when given', () => {
  const known = [{ id: 'telegram', name: 'Telegram (beta)' }];

  assert.strictEqual(Rail.accountDescription({ service: 'telegram', name: 'Work', detail: '' }, known), 'Telegram (beta) · Work');
});

test('standard keeps chats active within a month and the open one, folding an old chat even when it is unread', () => {
  const day = 24 * 60 * 60 * 1000;
  const now = 100 * day;
  const conversations = [
    { id: 'new', lastActivity: now - 2 * day, unread: 0, archived: false, hidden: false },
    { id: 'old', lastActivity: now - 40 * day, unread: 0, archived: false, hidden: false },
    { id: 'old-unread', lastActivity: now - 400 * day, unread: 2, archived: false, hidden: false },
    { id: 'old-open', lastActivity: now - 400 * day, unread: 0, archived: false, hidden: false },
    { id: 'never', lastActivity: 0, unread: 0, archived: false, hidden: false }
  ];

  assert.deepEqual(Rail.standard(conversations, now, 'old-open').map((c) => c.id), ['new', 'old-open']);
});

test('standard drops an archived conversation at once, even the open one', () => {
  const conversations = [
    { id: 'plain', archived: false, hidden: false, lastActivity: Date.now(), unread: 0 },
    { id: 'filed', archived: true, hidden: false, lastActivity: Date.now(), unread: 0 },
    { id: 'filed-open', archived: true, hidden: false, lastActivity: Date.now(), unread: 0 }
  ];

  assert.deepEqual(Rail.standard(conversations, Date.now(), 'filed-open').map((c) => c.id), ['plain']);
});

test('standard drops a hidden conversation at once, even the open one, recent or unread', () => {
  const conversations = [
    { id: 'plain', archived: false, hidden: false, lastActivity: Date.now(), unread: 0 },
    { id: 'hidden-unread', archived: false, hidden: true, lastActivity: Date.now(), unread: 3 },
    { id: 'hidden-open', archived: false, hidden: true, lastActivity: Date.now(), unread: 0 }
  ];

  assert.deepEqual(Rail.standard(conversations, Date.now(), 'hidden-open').map((c) => c.id), ['plain']);
});

test('standard: sending in an old chat brings it back once its activity is recent again', () => {
  const day = 24 * 60 * 60 * 1000;
  const now = 100 * day;
  const old = { id: 'chat', archived: false, hidden: false, lastActivity: now - 400 * day, unread: 0 };
  assert.deepEqual(Rail.standard([old], now, ''), []);

  const justSent = Object.assign({}, old, { lastActivity: now });
  assert.deepEqual(Rail.standard([justSent], now, '').map((c) => c.id), ['chat']);
});

test('standard: sending in a hidden chat keeps it hidden even once its activity is recent', () => {
  const now = Date.now();
  const sent = { id: 'chat', archived: false, hidden: true, lastActivity: now, unread: 0 };

  assert.deepEqual(Rail.standard([sent], now, ''), []);
});

test('isDimmed: true for anything the standard list would not show; the open chat is exempt only from being merely old', () => {
  const now = Date.now();
  const old = { id: 'old', archived: false, hidden: false, lastActivity: 0, unread: 0 };
  const hidden = { id: 'hidden', archived: false, hidden: true, lastActivity: now, unread: 0 };

  assert.strictEqual(Rail.isDimmed(old, now, ''), true);
  assert.strictEqual(Rail.isDimmed(old, now, 'old'), false);
  assert.strictEqual(Rail.isDimmed(hidden, now, ''), true);
  assert.strictEqual(Rail.isDimmed(hidden, now, 'hidden'), true);
  assert.strictEqual(Rail.isDimmed({ id: 'plain', archived: false, hidden: false, lastActivity: now, unread: 0 }, now, ''), false);
});

test('isDimmed: an old chat is dimmed even while unread, and carries no label since it is merely old', () => {
  const now = Date.now();
  const oldUnread = { id: 'old-unread', archived: false, hidden: false, lastActivity: 0, unread: 5 };

  assert.strictEqual(Rail.isDimmed(oldUnread, now, ''), true);
  assert.strictEqual(Rail.dimLabel(oldUnread, now, ''), '');
});

test('dimLabel names hidden or archived even for the open chat, and is blank for a merely older or non-dimmed chat', () => {
  const now = Date.now();
  const hidden = { id: 'h', archived: false, hidden: true, lastActivity: now, unread: 0 };
  const archived = { id: 'a', archived: true, hidden: false, lastActivity: now, unread: 0 };
  const old = { id: 'o', archived: false, hidden: false, lastActivity: 0, unread: 0 };
  const plain = { id: 'p', archived: false, hidden: false, lastActivity: now, unread: 0 };

  assert.strictEqual(Rail.dimLabel(hidden, now, ''), 'Hidden');
  assert.strictEqual(Rail.dimLabel(archived, now, ''), 'Archived');
  assert.strictEqual(Rail.dimLabel(old, now, ''), '');
  assert.strictEqual(Rail.dimLabel(plain, now, ''), '');
  assert.strictEqual(Rail.dimLabel(hidden, now, 'h'), 'Hidden');
  assert.strictEqual(Rail.dimLabel(old, now, 'o'), '');
});

test('compareConversations puts pinned conversations first, then newest activity', () => {
  const a = { id: 'a', pinned: false, lastActivity: 200 };
  const b = { id: 'b', pinned: true, lastActivity: 100 };
  const c = { id: 'c', pinned: false, lastActivity: 300 };
  const d = { id: 'd', pinned: true, lastActivity: 400 };

  const sorted = [a, b, c, d].sort(Rail.compareConversations).map((x) => x.id);

  assert.deepEqual(sorted, ['d', 'b', 'c', 'a']);
});
