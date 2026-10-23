'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Settings = load('lib/Settings.js');

const DEFAULT_SETTINGS = {
  notifications: true, notificationDetail: 'none', readReceipts: true,
  spellCheck: true, spellLanguage: 'auto',
};

test('withDefaults uses the manifest defaults when nothing was changed', () => {
  // Omarchy passes only the settings the user changed, so an untouched
  // widget sends {} and must not switch notifications off, or turn
  // incognito read receipts on.
  assert.deepEqual(Settings.withDefaults({}), DEFAULT_SETTINGS);
  assert.deepEqual(Settings.withDefaults(undefined), DEFAULT_SETTINGS);
});

test('withDefaults keeps the values the user changed', () => {
  const got = Settings.withDefaults({
    notifications: false, notificationDetail: 'Name only', readReceipts: false,
    spellCheck: false, spellLanguage: 'British English', unrelated: 'x',
  });

  assert.deepEqual(got, {
    notifications: false, notificationDetail: 'nameOnly', readReceipts: false,
    spellCheck: false, spellLanguage: 'en_GB',
  });
});

test('withDefaults falls back to the current readReceipts value, not the manifest default, when the caller passes one', () => {
  // Service.qml passes its own current value here so the palette's
  // "Toggle read receipts" command survives Omarchy re-forwarding the
  // bar widget's settings afterwards; see ui/Service.qml.
  assert.strictEqual(Settings.withDefaults({}, false).readReceipts, false);
  assert.strictEqual(Settings.withDefaults({}, true).readReceipts, true);
  // An explicit value from Omarchy itself still wins over that fallback.
  assert.strictEqual(Settings.withDefaults({ readReceipts: true }, false).readReceipts, true);
});

test('withDefaults translates every manifest label to the helper value', () => {
  assert.strictEqual(Settings.withDefaults({ notificationDetail: 'Name and message' }).notificationDetail, 'nameAndMessage');
  assert.strictEqual(Settings.withDefaults({ notificationDetail: 'Name only' }).notificationDetail, 'nameOnly');
  assert.strictEqual(Settings.withDefaults({ notificationDetail: 'Nothing' }).notificationDetail, 'none');
});

test('withDefaults passes through a helper value already in that form', () => {
  assert.strictEqual(Settings.withDefaults({ notificationDetail: 'none' }).notificationDetail, 'none');
});

test('withDefaults migrates the older notificationPreview boolean when no detail was sent', () => {
  assert.strictEqual(Settings.withDefaults({ notificationPreview: true }).notificationDetail, 'nameAndMessage');
  assert.strictEqual(Settings.withDefaults({ notificationPreview: false }).notificationDetail, 'nameOnly');
});

test('withDefaults prefers notificationDetail over notificationPreview when both are sent', () => {
  const got = Settings.withDefaults({ notificationPreview: false, notificationDetail: 'Nothing' });
  assert.strictEqual(got.notificationDetail, 'none');
});

test('withDefaults translates every manifest spellLanguage label to the helper value', () => {
  assert.strictEqual(Settings.withDefaults({ spellLanguage: 'Automatic' }).spellLanguage, 'auto');
  assert.strictEqual(Settings.withDefaults({ spellLanguage: 'British English' }).spellLanguage, 'en_GB');
  assert.strictEqual(Settings.withDefaults({ spellLanguage: 'American English' }).spellLanguage, 'en_US');
});

test('withDefaults passes through a spellLanguage helper value already in that form', () => {
  assert.strictEqual(Settings.withDefaults({ spellLanguage: 'en_GB' }).spellLanguage, 'en_GB');
});

test('withDefaults falls back to the manifest default for an unknown spellLanguage', () => {
  assert.strictEqual(Settings.withDefaults({ spellLanguage: 'Klingon' }).spellLanguage, 'auto');
});
