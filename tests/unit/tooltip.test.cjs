// Tests for ui/lib/Tooltip.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Tooltip = load('lib/Tooltip.js');

test('format shows just the total with nothing else to report', () => {
  assert.strictEqual(Tooltip.format({ total: 0 }), 'OmaMessenger · 0 unread');
});

test('format lines up each service with unread messages, sorted by id', () => {
  const text = Tooltip.format({
    total: 7,
    byService: { telegram: 2, whatsapp: 5 }
  });
  assert.strictEqual(text, 'OmaMessenger · 7 unread\nTelegram: 2 unread\nWhatsApp: 5 unread');
});

test('format leaves out a service with no unread messages', () => {
  const text = Tooltip.format({ total: 5, byService: { whatsapp: 5 } });
  assert.strictEqual(text, 'OmaMessenger · 5 unread\nWhatsApp: 5 unread');
});

test('format uses the helper\'s own service names when it names one', () => {
  const text = Tooltip.format({
    total: 1,
    byService: { whatsapp: 1 },
    services: [{ id: 'whatsapp', name: 'WhatsApp Business' }]
  });
  assert.strictEqual(text, 'OmaMessenger · 1 unread\nWhatsApp Business: 1 unread');
});

test('format falls back to the service id for one it does not know', () => {
  const text = Tooltip.format({ total: 1, byService: { signal: 1 } });
  assert.strictEqual(text, 'OmaMessenger · 1 unread\nsignal: 1 unread');
});

test('format adds a line for each account that is not connected', () => {
  const text = Tooltip.format({
    total: 0,
    accounts: [
      { service: 'whatsapp', name: 'Personal', status: 'connected' },
      { service: 'telegram', name: 'Work', status: 'needs-auth' }
    ]
  });
  assert.strictEqual(text, 'OmaMessenger · 0 unread\nTelegram (Work): needs sign-in');
});

test('format describes every known disconnected status', () => {
  const statuses = ['connecting', 'needs-auth', 'error', 'offline'];
  for (const status of statuses) {
    const text = Tooltip.format({
      total: 0,
      accounts: [{ service: 'whatsapp', name: 'Personal', status }]
    });
    assert.ok(text.endsWith(': ' + (status === 'needs-auth' ? 'needs sign-in' : status)), text);
  }
});

test('format falls back to the raw status for one it does not know', () => {
  const text = Tooltip.format({
    total: 0,
    accounts: [{ service: 'whatsapp', name: 'Personal', status: 'syncing' }]
  });
  assert.strictEqual(text, 'OmaMessenger · 0 unread\nWhatsApp (Personal): syncing');
});

test('format combines per-service unread and disconnected accounts', () => {
  const text = Tooltip.format({
    total: 5,
    byService: { whatsapp: 5 },
    accounts: [
      { service: 'whatsapp', name: 'Personal', status: 'connected' },
      { service: 'telegram', name: 'Work', status: 'offline' }
    ]
  });
  assert.strictEqual(
    text,
    'OmaMessenger · 5 unread\nWhatsApp: 5 unread\nTelegram (Work): offline'
  );
});

// Every test above has run once with a deliberately broken expectation
// (comparing against the wrong joined string) to see it fail before this
// was written.
