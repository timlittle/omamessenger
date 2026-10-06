'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Settings = load('lib/Settings.js');

test('withDefaults turns everything on when nothing was changed', () => {
  // Omarchy passes only the settings the user changed, so an untouched
  // widget sends {} and must not switch notifications off.
  assert.deepEqual(Settings.withDefaults({}), { notifications: true, notificationPreview: true, demoChatter: true });
  assert.deepEqual(Settings.withDefaults(undefined), { notifications: true, notificationPreview: true, demoChatter: true });
});

test('withDefaults keeps the values the user changed', () => {
  const got = Settings.withDefaults({ notificationPreview: false, unrelated: 'x' });

  assert.deepEqual(got, { notifications: true, notificationPreview: false, demoChatter: true });
});
