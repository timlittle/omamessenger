'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const KeyBindings = load('lib/KeyBindings.js');
const Keymap = load('lib/Keymap.js');

test('configPath prefers XDG_CONFIG_HOME, falling back to home/.config', () => {
  assert.strictEqual(KeyBindings.configPath('/home/x', ''), '/home/x/.config/omamessenger/keys.conf');
  assert.strictEqual(KeyBindings.configPath('/home/x', '/custom/config'), '/custom/config/omamessenger/keys.conf');
});

test('parseConfig skips blank lines and whole-line comments', () => {
  const { overrides, errors } = KeyBindings.parseConfig('\n# a comment\n   \n# unread.next = Ctrl+J\n');

  assert.deepEqual(overrides, {});
  assert.deepEqual(errors, []);
});

test('parseConfig reads one override, trimming whitespace around it', () => {
  const { overrides, errors } = KeyBindings.parseConfig('  unread.next = Ctrl+Alt+P  \n');

  assert.deepEqual(overrides, { 'unread.next': ['Ctrl+Alt+P'] });
  assert.deepEqual(errors, []);
});

test('parseConfig reads several comma-separated keys for one action', () => {
  const { overrides } = KeyBindings.parseConfig('unread.next = Ctrl+Alt+P, Ctrl+Alt+O\n');

  assert.deepEqual(overrides, { 'unread.next': ['Ctrl+Alt+P', 'Ctrl+Alt+O'] });
});

test('parseConfig reports an unknown action, not fatal, and skips it', () => {
  const { overrides, errors } = KeyBindings.parseConfig('bogus.action = Ctrl+K\nunread.next = Ctrl+Alt+P\n');

  assert.deepEqual(overrides, { 'unread.next': ['Ctrl+Alt+P'] });
  assert.strictEqual(errors.length, 1);
  assert.strictEqual(errors[0].line, 1);
  assert.match(errors[0].message, /unknown action "bogus\.action"/);
});

test('parseConfig reports a line with no "="', () => {
  const { errors } = KeyBindings.parseConfig('this is not a line\n');

  assert.strictEqual(errors.length, 1);
  assert.match(errors[0].message, /missing "="/);
});

test('parseConfig reports an action given no valid keys', () => {
  const { overrides, errors } = KeyBindings.parseConfig('unread.next =\n');

  assert.deepEqual(overrides, {});
  assert.strictEqual(errors.length, 1);
  assert.match(errors[0].message, /no valid keys/);
});

test('parseConfig keeps the valid keys on a line and reports only the malformed one', () => {
  const { overrides, errors } = KeyBindings.parseConfig('unread.next = Ctrl+J, NotAKey\n');

  assert.deepEqual(overrides, { 'unread.next': ['Ctrl+J'] });
  assert.strictEqual(errors.length, 1);
  assert.match(errors[0].message, /malformed key "NotAKey"/);
});

test('parseConfig reports a line number per error', () => {
  const { errors } = KeyBindings.parseConfig('# comment\nbogus.one = Ctrl+K\nbogus.two = Ctrl+L\n');

  assert.deepEqual(errors.map((e) => e.line), [2, 3]);
});

test('merge applies a clean override with no conflict', () => {
  const { bindings, conflicts } = KeyBindings.merge(Keymap.BINDINGS, { 'unread.next': ['Ctrl+Alt+P'] });

  const unreadNext = bindings.find((b) => b.action === 'unread.next');
  assert.deepEqual(unreadNext.keys, ['Ctrl+Alt+P']);
  assert.strictEqual(unreadNext.overridden, true);
  assert.deepEqual(conflicts, []);

  // Every other binding keeps its own default and is marked unoverridden.
  const cursorDown = bindings.find((b) => b.action === 'cursor.down');
  assert.deepEqual(cursorDown.keys, ['j', 'Down']);
  assert.strictEqual(cursorDown.overridden, false);
});

test('merge reverts two overrides that collide with each other, and reports it', () => {
  const overrides = { 'cursor.down': ['x'], 'cursor.up': ['x'] };
  const { bindings, conflicts } = KeyBindings.merge(Keymap.BINDINGS, overrides);

  const cursorDown = bindings.find((b) => b.action === 'cursor.down');
  const cursorUp = bindings.find((b) => b.action === 'cursor.up');
  assert.deepEqual(cursorDown.keys, ['j', 'Down']);
  assert.strictEqual(cursorDown.overridden, false);
  assert.deepEqual(cursorUp.keys, ['k', 'Up']);
  assert.strictEqual(cursorUp.overridden, false);

  assert.strictEqual(conflicts.length, 1);
  assert.strictEqual(conflicts[0].context, 'list');
  assert.deepEqual(conflicts[0].actions, ['cursor.down', 'cursor.up']);
});

test('merge reverts only the overridden side of a conflict with an untouched default', () => {
  // message.react defaults to "e"; message.delete defaults to "d". Moving
  // react onto delete's own key must revert react, and leave delete alone.
  const { bindings, conflicts } = KeyBindings.merge(Keymap.BINDINGS, { 'message.react': ['d'] });

  const react = bindings.find((b) => b.action === 'message.react');
  const del = bindings.find((b) => b.action === 'message.delete');
  assert.deepEqual(react.keys, ['e']);
  assert.strictEqual(react.overridden, false);
  assert.deepEqual(del.keys, ['d']);
  assert.strictEqual(del.overridden, false);

  assert.strictEqual(conflicts.length, 1);
  assert.deepEqual(conflicts[0].actions, ['message.delete', 'message.react']);
});

test('merge does not flag a global override sharing a key with a context-specific default', () => {
  // rail.all is global; cursor.top is list-only and already uses "g".
  // Keymap.match always resolves a context's own binding before a global
  // one, so cursor.top deterministically shadows rail.all inside "list"
  // without either ever being ambiguous; overriding rail.all onto "g" is
  // allowed to stand; it only answers to "g" outside the list context.
  const { bindings, conflicts } = KeyBindings.merge(Keymap.BINDINGS, { 'rail.all': ['g'] });

  const railAll = bindings.find((b) => b.action === 'rail.all');
  assert.deepEqual(railAll.keys, ['g']);
  assert.strictEqual(railAll.overridden, true);
  assert.deepEqual(conflicts, []);
});

test('merge treats an equivalent respelling of the same press as the same key', () => {
  // "Shift+G" and the bare uppercase "G" both mean the same physical
  // press; overriding one action onto it must still conflict with the
  // other action that already uses it by its usual spelling.
  const { conflicts } = KeyBindings.merge(Keymap.BINDINGS, { 'cursor.down': ['Shift+G'] });

  assert.strictEqual(conflicts.length, 1);
  assert.deepEqual(conflicts[0].actions, ['cursor.bottom', 'cursor.down']);
});

test('template lists every action, commented out, with its default keys', () => {
  const text = KeyBindings.template(Keymap.BINDINGS);

  assert.match(text, /^# OmaMessenger key bindings\./);
  assert.match(text, /# unread\.next = Ctrl\+J, Alt\+Shift\+Down {2}-- Next unread conversation/);
  assert.match(text, /# account\.add =\s+-- Add an account/);
});

test('rows lists errors and conflicts before the effective bindings, grouped by context', () => {
  const errors = [{ line: 3, message: 'unknown action "x"' }];
  const conflicts = [{ context: 'list', key: 'g', actions: ['cursor.down', 'cursor.top'] }];
  const { bindings } = KeyBindings.merge(Keymap.BINDINGS, {});

  const rows = KeyBindings.rows(bindings, conflicts, errors);

  assert.strictEqual(rows[0].label, 'Error (line 3)');
  assert.strictEqual(rows[1].label, 'Conflict: cursor.down, cursor.top');
  assert.ok(rows.some((r) => r.label === '— global —'));
  const unreadRow = rows.find((r) => r.label === 'Next unread conversation');
  assert.strictEqual(unreadRow.detail, 'default');
});

test('rows marks an overridden action as custom', () => {
  const { bindings } = KeyBindings.merge(Keymap.BINDINGS, { 'unread.next': ['Ctrl+Alt+P'] });
  const rows = KeyBindings.rows(bindings, [], []);

  const unreadRow = rows.find((r) => r.label === 'Next unread conversation');
  assert.strictEqual(unreadRow.detail, 'custom');
  assert.strictEqual(unreadRow.keys, 'Ctrl+Alt+P');
});

test('reportText omits empty sections and lists bindings by context', () => {
  const { bindings } = KeyBindings.merge(Keymap.BINDINGS, {});
  const text = KeyBindings.reportText(bindings, [], []);

  assert.ok(!text.includes('Errors:'));
  assert.ok(!text.includes('Conflicts'));
  assert.match(text, /\[global\]/);
  assert.match(text, /unread\.next: Ctrl\+J, Alt\+Shift\+↓/);
});

test('reportText includes errors and conflicts when there are any', () => {
  const errors = [{ line: 1, message: 'unknown action "x"' }];
  const conflicts = [{ context: 'list', key: 'g', actions: ['cursor.down', 'cursor.top'] }];
  const { bindings } = KeyBindings.merge(Keymap.BINDINGS, {});

  const text = KeyBindings.reportText(bindings, conflicts, errors);

  assert.match(text, /Errors:\n {2}line 1: unknown action "x"/);
  assert.match(text, /Conflicts \(default wins\):\n {2}list: cursor\.down, cursor\.top both want g/);
});
