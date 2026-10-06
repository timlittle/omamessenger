'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Keymap = load('lib/Keymap.js');
const { KEY, MOD } = Keymap;

// letter returns the Qt key code for a letter or digit.
const letter = (ch) => ch.toUpperCase().charCodeAt(0);

// Each row: context, key, modifiers, typed text, demo mode, expected action.
const cases = [
  ['list', KEY.Slash, MOD.Ctrl, '', false, 'palette.commands'],
  ['list', KEY.Question, MOD.Ctrl | MOD.Shift, '', false, 'palette.commands'],
  ['compose', letter('p'), MOD.Ctrl | MOD.Shift, '', false, 'palette.commands'],
  ['list', letter('k'), MOD.Ctrl, '', false, 'palette.conversations'],
  ['conversation', letter('t'), MOD.Ctrl, '', false, 'palette.conversations'],
  ['list', letter('g'), MOD.Ctrl, '', false, 'search.focus'],
  ['list', letter('n'), MOD.Ctrl, '', false, 'chat.new'],
  ['list', letter('k'), MOD.Ctrl | MOD.Shift, '', false, 'chat.new'],
  ['conversation', letter('j'), MOD.Ctrl, '', false, 'unread.next'],
  ['list', KEY.Down, MOD.Alt | MOD.Shift, '', false, 'unread.next'],
  ['list', KEY.Up, MOD.Alt | MOD.Shift, '', false, 'unread.prev'],
  ['compose', KEY.Down, MOD.Alt, '', false, 'chat.next'],
  ['compose', KEY.Up, MOD.Alt, '', false, 'chat.prev'],
  ['list', letter('0'), MOD.Ctrl, '', false, 'rail.all'],
  ['list', letter('1'), MOD.Ctrl, '', false, 'rail.whatsapp'],
  ['list', letter('2'), MOD.Ctrl, '', false, 'rail.telegram'],
  ['list', KEY.Tab, MOD.Ctrl, '', false, 'rail.next'],
  ['list', KEY.Backtab, MOD.Ctrl | MOD.Shift, '', false, 'rail.prev'],
  ['list', letter('w'), MOD.Ctrl, '', false, 'window.hide'],
  ['compose', letter('q'), MOD.Ctrl, '', false, 'app.quit'],
  ['list', letter('d'), MOD.Ctrl | MOD.Shift, '', true, 'demo.inject'],
  ['list', letter('d'), MOD.Ctrl | MOD.Shift, '', false, ''],
  ['compose', KEY.Escape, 0, '', false, 'escape'],

  ['list', letter('j'), 0, 'j', false, 'cursor.down'],
  ['list', KEY.Down, 0, '', false, 'cursor.down'],
  ['list', letter('k'), 0, 'k', false, 'cursor.up'],
  ['list', letter('g'), 0, 'g', false, 'cursor.top'],
  ['list', letter('g'), MOD.Shift, 'G', false, 'cursor.bottom'],
  ['list', KEY.Return, 0, '\r', false, 'chat.open'],
  ['list', KEY.Enter, 0, '\r', false, 'chat.open'],
  ['list', KEY.Tab, 0, '\t', false, 'pane.conversation'],
  ['list', letter('m'), 0, 'm', false, 'chat.mute'],

  ['conversation', letter('j'), 0, 'j', false, 'scroll.down'],
  ['conversation', letter('d'), MOD.Ctrl, '', false, 'scroll.pageDown'],
  ['conversation', letter('g'), MOD.Shift, 'G', false, 'scroll.newest'],
  ['conversation', KEY.Return, 0, '\r', false, 'compose.focus'],
  ['conversation', letter('h'), 0, 'h', false, 'pane.list'],
  ['conversation', letter('r'), 0, 'r', false, 'message.retry'],

  ['compose', KEY.Return, 0, '\r', false, 'message.send'],
  ['compose', KEY.Return, MOD.Shift, '\r', false, ''],
  ['compose', letter('j'), 0, 'j', false, ''],
  ['search', KEY.Return, 0, '\r', false, 'search.accept'],
  ['search', letter('j'), 0, 'j', false, ''],

  ['dialog', letter('k'), MOD.Ctrl, '', false, 'dialog.up'],
  ['dialog', KEY.Tab, MOD.Ctrl, '', false, 'dialog.nextAccount'],
  ['palette', letter('k'), MOD.Ctrl, '', false, 'palette.up'],
  ['palette', letter('j'), MOD.Ctrl, '', false, 'palette.down'],
  ['palette', letter('n'), MOD.Ctrl, '', false, 'palette.down'],
  ['palette', KEY.Return, 0, '\r', false, 'palette.accept'],
  ['palette', letter('j'), 0, 'j', false, '']
];

for (const [context, key, modifiers, text, demo, want] of cases) {
  test(`match in ${context}: key ${key.toString(16)} mods ${modifiers.toString(16)} → ${want || 'nothing'}`, () => {
    assert.strictEqual(Keymap.match(context, key, modifiers, text, demo), want);
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
});

test('the palette offers commands with their first key, and demo ones only in demo', () => {
  const real = Keymap.commands(false);
  const demo = Keymap.commands(true);

  const jump = real.find((c) => c.action === 'palette.conversations');
  assert.deepEqual(jump, { action: 'palette.conversations', label: 'Jump to conversation', keys: 'Ctrl+K' });
  assert.ok(!real.some((c) => c.action === 'demo.inject'));
  assert.ok(demo.some((c) => c.action === 'demo.inject'));
  assert.ok(!real.some((c) => c.action === 'palette.commands' || c.action === 'cursor.down'));
});

test('bindingsFor puts the context hints before the global ones', () => {
  const actions = Keymap.bindingsFor('list').map((b) => b.action);

  assert.strictEqual(actions[0], 'chat.open');
  assert.ok(actions.includes('palette.commands'));
  assert.ok(!actions.includes('message.send'));
});
