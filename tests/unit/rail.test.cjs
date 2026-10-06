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

test('accountLabel says which service an account belongs to', () => {
  assert.strictEqual(Rail.accountLabel({ service: 'telegram', name: 'Work' }), 'Telegram · Work');
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
