'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Keymap = load('lib/Keymap.js');
const { KEY, MOD } = Keymap;

// letter returns the Qt key code for a letter or digit.
const letter = (ch) => ch.toUpperCase().charCodeAt(0);

// Each row: context, key, modifiers, typed text, expected action.
const cases = [
  ['list', KEY.Slash, MOD.Ctrl, '', 'palette.commands'],
  ['list', KEY.Question, MOD.Ctrl | MOD.Shift, '', 'palette.commands'],
  ['compose', letter('p'), MOD.Ctrl | MOD.Shift, '', 'palette.commands'],
  ['list', letter('k'), MOD.Ctrl, '', 'palette.conversations'],
  ['conversation', letter('t'), MOD.Ctrl, '', 'palette.conversations'],
  ['list', letter('g'), MOD.Ctrl, '', 'search.focus'],
  ['list', letter('n'), MOD.Ctrl, '', 'chat.new'],
  ['list', letter('k'), MOD.Ctrl | MOD.Shift, '', 'chat.new'],
  ['conversation', letter('j'), MOD.Ctrl, '', 'unread.next'],
  ['list', KEY.Down, MOD.Alt | MOD.Shift, '', 'unread.next'],
  ['list', KEY.Up, MOD.Alt | MOD.Shift, '', 'unread.prev'],
  ['compose', KEY.Down, MOD.Alt, '', 'chat.next'],
  ['compose', KEY.Up, MOD.Alt, '', 'chat.prev'],
  ['list', letter('0'), MOD.Ctrl, '', 'rail.all'],
  ['list', letter('1'), MOD.Ctrl, '', 'rail.whatsapp'],
  ['list', letter('2'), MOD.Ctrl, '', 'rail.telegram'],
  ['list', KEY.Tab, MOD.Ctrl, '', 'rail.next'],
  ['list', KEY.Backtab, MOD.Ctrl | MOD.Shift, '', 'rail.prev'],
  ['list', letter('w'), MOD.Ctrl, '', 'window.hide'],
  ['compose', letter('q'), MOD.Ctrl, '', 'app.quit'],
  ['compose', KEY.Escape, 0, '', 'escape'],

  ['list', letter('j'), 0, 'j', 'cursor.down'],
  ['list', KEY.Down, 0, '', 'cursor.down'],
  ['list', letter('k'), 0, 'k', 'cursor.up'],
  ['list', letter('g'), 0, 'g', 'cursor.top'],
  ['list', letter('g'), MOD.Shift, 'G', 'cursor.bottom'],
  ['list', KEY.Return, 0, '\r', 'chat.open'],
  ['list', KEY.Enter, 0, '\r', 'chat.open'],
  ['list', KEY.Tab, 0, '\t', 'pane.conversation'],
  ['list', letter('m'), 0, 'm', 'chat.mute'],

  ['conversation', letter('j'), 0, 'j', 'scroll.down'],
  ['conversation', letter('d'), MOD.Ctrl, '', 'scroll.pageDown'],
  ['conversation', letter('g'), MOD.Shift, 'G', 'scroll.newest'],
  ['conversation', KEY.Return, 0, '\r', 'compose.focus'],
  ['conversation', letter('h'), 0, 'h', 'pane.list'],
  ['conversation', letter('r'), 0, 'r', 'message.retry'],
  ['conversation', letter('r'), MOD.Shift, 'R', 'message.reply'],

  ['compose', KEY.Return, 0, '\r', 'message.send'],
  ['compose', KEY.Return, MOD.Shift, '\r', ''],
  ['compose', letter('j'), 0, 'j', ''],
  ['search', KEY.Return, 0, '\r', 'search.accept'],
  ['search', letter('j'), 0, 'j', ''],

  ['dialog', letter('k'), MOD.Ctrl, '', 'dialog.up'],
  ['dialog', KEY.Tab, MOD.Ctrl, '', 'dialog.nextAccount'],
  ['palette', letter('k'), MOD.Ctrl, '', 'palette.up'],
  ['palette', letter('j'), MOD.Ctrl, '', 'palette.down'],
  ['palette', letter('n'), MOD.Ctrl, '', 'palette.down'],
  ['palette', KEY.Return, 0, '\r', 'palette.accept'],
  ['palette', letter('j'), 0, 'j', ''],

  ['reactionPicker', KEY.Left, 0, '', 'reaction.left'],
  ['reactionPicker', KEY.Right, 0, '', 'reaction.right'],
  ['reactionPicker', KEY.Enter, 0, '\r', 'reaction.accept'],
  ['reactionPicker', KEY.Escape, 0, '', 'escape'],

  // The viewer and the reaction picker share Left/Right physically, but
  // never open at once (see Navigation.keyContext); each context only
  // answers to its own binding.
  ['viewer', KEY.Right, 0, '', 'viewer.next'],
  ['viewer', KEY.Left, 0, '', 'viewer.prev'],
  ['viewer', KEY.Escape, 0, '', 'escape'],
  ['viewer', KEY.Enter, 0, '\r', ''],
  ['list', KEY.Right, 0, '', '']
];

for (const [context, key, modifiers, text, want] of cases) {
  test(`match in ${context}: key ${key.toString(16)} mods ${modifiers.toString(16)} → ${want || 'nothing'}`, () => {
    assert.strictEqual(Keymap.match(context, key, modifiers, text), want);
  });
}

test('every global binding uses Ctrl or Alt, or is Escape, so typing is never stolen', () => {
  for (const binding of Keymap.BINDINGS.filter((b) => b.contexts.includes('global'))) {
    for (const spec of binding.keys) {
      assert.ok(/^(Ctrl|Alt)\+/.test(spec) || spec === 'Escape', `${binding.action}: ${spec}`);
    }
  }
});

test('display shows keys the way people read them', () => {
  assert.strictEqual(Keymap.display('Ctrl+Slash'), 'Ctrl+/');
  assert.strictEqual(Keymap.display('Alt+Shift+Down'), 'Alt+Shift+↓');
  assert.strictEqual(Keymap.display('Ctrl+K'), 'Ctrl+K');
  assert.strictEqual(Keymap.display('Left'), '←');
  assert.strictEqual(Keymap.display('Right'), '→');
});

test('bindingsFor hints the photo viewer steps without offering them as commands', () => {
  const actions = Keymap.bindingsFor('viewer').map((b) => b.action);

  assert.ok(actions.includes('viewer.next'));
  assert.ok(actions.includes('viewer.prev'));
  assert.ok(!Keymap.commands().some((c) => c.action === 'viewer.next'));
});

test('the palette offers commands with their first key', () => {
  const commands = Keymap.commands();

  const jump = commands.find((c) => c.action === 'palette.conversations');
  assert.deepEqual(jump, { action: 'palette.conversations', label: 'Jump to conversation', keys: 'Ctrl+K' });
  assert.ok(!commands.some((c) => c.action === 'palette.commands' || c.action === 'cursor.down'));
});

test('bindingsFor puts the context hints before the global ones', () => {
  const actions = Keymap.bindingsFor('list').map((b) => b.action);

  assert.strictEqual(actions[0], 'chat.open');
  assert.ok(actions.includes('palette.commands'));
  assert.ok(!actions.includes('message.send'));
});

test('the palette offers commands that have no key, with none shown', () => {
  const add = Keymap.commands().find((c) => c.action === 'account.add');

  assert.deepEqual(add, { action: 'account.add', label: 'Add an account', keys: '' });
  assert.ok(Keymap.commands().some((c) => c.action === 'account.addOwnKeys'));
  assert.ok(Keymap.commands().some((c) => c.action === 'account.remove'));
  assert.ok(Keymap.commands().some((c) => c.action === 'list.olderChats'));
});
