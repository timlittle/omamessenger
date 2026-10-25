'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const NotificationSound = load('lib/NotificationSound.js');

const resolveCases = [
  ['a conversation with its own sound always wins', true, '/sounds/whatsapp.ogg', '/sounds/default.ogg', { source: 'conversation' }],
  ['a conversation with its own sound wins even with nothing else set', true, '', '', { source: 'conversation' }],
  ['a service sound wins over the default when there is no conversation sound', false, '/sounds/whatsapp.ogg', '/sounds/default.ogg', { source: 'service', path: '/sounds/whatsapp.ogg' }],
  ['the global default plays when neither the conversation nor the service has one', false, '', '/sounds/default.ogg', { source: 'default', path: '/sounds/default.ogg' }],
  ['nothing plays when nothing anywhere is set', false, '', '', { source: 'silent' }],
  ['an explicit "none" service setting silences it, without falling back to the default', false, 'none', '/sounds/default.ogg', { source: 'silent' }],
  ['an explicit "default" service setting falls through to the global default', false, 'default', '/sounds/default.ogg', { source: 'default', path: '/sounds/default.ogg' }],
  ['a blank or whitespace-only service setting falls through to the default', false, '   ', '/sounds/default.ogg', { source: 'default', path: '/sounds/default.ogg' }],
  ['an explicit "none" default setting plays nothing', false, '', 'none', { source: 'silent' }],
  ['a missing (undefined) service or default setting is treated the same as ""', false, undefined, undefined, { source: 'silent' }],
  ['"NONE" in any case still silences it', false, 'NONE', '/sounds/default.ogg', { source: 'silent' }]
];

for (const [name, hasCustomSound, serviceSound, defaultSound, want] of resolveCases) {
  test(`resolve: ${name}`, () => {
    assert.deepEqual(NotificationSound.resolve(hasCustomSound, serviceSound, defaultSound), want);
  });
}
