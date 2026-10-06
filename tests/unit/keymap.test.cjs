// Tests for ui/lib/Keymap.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Keymap = load('lib/Keymap.js');

test('match returns empty string for unknown key', () => {
  assert.strictEqual(Keymap.match('list', 0xFFFF, 0, '', false), '');
});

test('match returns empty string for invalid context', () => {
  assert.strictEqual(Keymap.match('invalid', Keymap.KEY.Return, 0, '', false), '');
});

// Global bindings: should work in any context.
test('match: Ctrl+K in global context returns search.focus', () => {
  assert.strictEqual(Keymap.match('global', 0x4B, Keymap.MOD.Ctrl, '', false), 'search.focus');
});

test('match: Ctrl+K in list context returns search.focus', () => {
  assert.strictEqual(Keymap.match('list', 0x4B, Keymap.MOD.Ctrl, '', false), 'search.focus');
});

test('match: Ctrl+K in conversation context returns search.focus', () => {
  assert.strictEqual(Keymap.match('conversation', 0x4B, Keymap.MOD.Ctrl, '', false), 'search.focus');
});

test('match: Ctrl+N returns chat.new in any context', () => {
  assert.strictEqual(Keymap.match('list', 0x4E, Keymap.MOD.Ctrl, '', false), 'chat.new');
});

test('match: Ctrl+0 returns rail.all', () => {
  assert.strictEqual(Keymap.match('global', 0x30, Keymap.MOD.Ctrl, '', false), 'rail.all');
});

test('match: Ctrl+1 returns rail.whatsapp', () => {
  assert.strictEqual(Keymap.match('global', 0x31, Keymap.MOD.Ctrl, '', false), 'rail.whatsapp');
});

test('match: Ctrl+2 returns rail.telegram', () => {
  assert.strictEqual(Keymap.match('global', 0x32, Keymap.MOD.Ctrl, '', false), 'rail.telegram');
});

test('match: Ctrl+Tab returns rail.next', () => {
  assert.strictEqual(Keymap.match('global', Keymap.KEY.Tab, Keymap.MOD.Ctrl, '', false), 'rail.next');
});

test('match: Ctrl+Shift+Tab returns rail.prev', () => {
  assert.strictEqual(
    Keymap.match('global', Keymap.KEY.Tab, Keymap.MOD.Ctrl | Keymap.MOD.Shift, '', false),
    'rail.prev'
  );
});

test('match: F1 returns help.toggle in global context', () => {
  assert.strictEqual(Keymap.match('global', Keymap.KEY.F1, 0, '', false), 'help.toggle');
});

test('match: Ctrl+Shift+D demo.inject only when demo is true', () => {
  assert.strictEqual(
    Keymap.match('global', 0x44, Keymap.MOD.Ctrl | Keymap.MOD.Shift, '', false),
    ''
  );
  assert.strictEqual(
    Keymap.match('global', 0x44, Keymap.MOD.Ctrl | Keymap.MOD.Shift, '', true),
    'demo.inject'
  );
});

test('match: Escape returns escape', () => {
  assert.strictEqual(Keymap.match('global', Keymap.KEY.Escape, 0, '', false), 'escape');
});

// List context bindings.
test('match: j in list returns cursor.down', () => {
  assert.strictEqual(Keymap.match('list', 0x4A, 0, '', false), 'cursor.down');
});

test('match: k in list returns cursor.up', () => {
  assert.strictEqual(Keymap.match('list', 0x4B, 0, '', false), 'cursor.up');
});

test('match: Down in list returns cursor.down', () => {
  assert.strictEqual(Keymap.match('list', Keymap.KEY.Down, 0, '', false), 'cursor.down');
});

test('match: Up in list returns cursor.up', () => {
  assert.strictEqual(Keymap.match('list', Keymap.KEY.Up, 0, '', false), 'cursor.up');
});

test('match: g in list returns cursor.top', () => {
  assert.strictEqual(Keymap.match('list', 0x47, 0, '', false), 'cursor.top');
});

test('match: G (Shift+G) in list returns cursor.bottom', () => {
  assert.strictEqual(Keymap.match('list', 0x47, Keymap.MOD.Shift, '', false), 'cursor.bottom');
});

test('match: Home in list returns cursor.top', () => {
  assert.strictEqual(Keymap.match('list', Keymap.KEY.Home, 0, '', false), 'cursor.top');
});

test('match: End in list returns cursor.bottom', () => {
  assert.strictEqual(Keymap.match('list', Keymap.KEY.End, 0, '', false), 'cursor.bottom');
});

test('match: Enter in list returns chat.open', () => {
  assert.strictEqual(Keymap.match('list', Keymap.KEY.Return, 0, '', false), 'chat.open');
});

test('match: l in list returns chat.open', () => {
  assert.strictEqual(Keymap.match('list', 0x4C, 0, '', false), 'chat.open');
});

test('match: o in list returns chat.open', () => {
  assert.strictEqual(Keymap.match('list', 0x4F, 0, '', false), 'chat.open');
});

test('match: i in list returns chat.open', () => {
  assert.strictEqual(Keymap.match('list', 0x49, 0, '', false), 'chat.open');
});

test('match: Tab in list returns pane.conversation', () => {
  assert.strictEqual(Keymap.match('list', Keymap.KEY.Tab, 0, '', false), 'pane.conversation');
});

test('match: m in list returns chat.mute', () => {
  assert.strictEqual(Keymap.match('list', 0x4D, 0, '', false), 'chat.mute');
});

test('match: u in list returns unread.next', () => {
  assert.strictEqual(Keymap.match('list', 0x55, 0, '', false), 'unread.next');
});

test('match: / in list returns search.focus', () => {
  assert.strictEqual(Keymap.match('list', 0, 0, '/', false), 'search.focus');
});

test('match: ? (Shift+/) in list returns help.toggle', () => {
  assert.strictEqual(Keymap.match('list', 0, Keymap.MOD.Shift, '?', false), 'help.toggle');
});

test('match: q in list returns window.hide', () => {
  assert.strictEqual(Keymap.match('list', 0x51, 0, '', false), 'window.hide');
});

// Conversation context bindings.
test('match: j in conversation returns scroll.down', () => {
  assert.strictEqual(Keymap.match('conversation', 0x4A, 0, '', false), 'scroll.down');
});

test('match: k in conversation returns scroll.up', () => {
  assert.strictEqual(Keymap.match('conversation', 0x4B, 0, '', false), 'scroll.up');
});

test('match: Ctrl+D in conversation returns scroll.pageDown', () => {
  assert.strictEqual(Keymap.match('conversation', 0x44, Keymap.MOD.Ctrl, '', false), 'scroll.pageDown');
});

test('match: PageDown in conversation returns scroll.pageDown', () => {
  assert.strictEqual(Keymap.match('conversation', Keymap.KEY.PageDown, 0, '', false), 'scroll.pageDown');
});

test('match: Ctrl+U in conversation returns scroll.pageUp', () => {
  assert.strictEqual(Keymap.match('conversation', 0x55, Keymap.MOD.Ctrl, '', false), 'scroll.pageUp');
});

test('match: PageUp in conversation returns scroll.pageUp', () => {
  assert.strictEqual(Keymap.match('conversation', Keymap.KEY.PageUp, 0, '', false), 'scroll.pageUp');
});

test('match: G in conversation returns scroll.newest', () => {
  assert.strictEqual(Keymap.match('conversation', 0x47, Keymap.MOD.Shift, '', false), 'scroll.newest');
});

test('match: End in conversation returns scroll.newest', () => {
  assert.strictEqual(Keymap.match('conversation', Keymap.KEY.End, 0, '', false), 'scroll.newest');
});

test('match: g in conversation returns scroll.oldest', () => {
  assert.strictEqual(Keymap.match('conversation', 0x47, 0, '', false), 'scroll.oldest');
});

test('match: Home in conversation returns scroll.oldest', () => {
  assert.strictEqual(Keymap.match('conversation', Keymap.KEY.Home, 0, '', false), 'scroll.oldest');
});

test('match: i in conversation returns compose.focus', () => {
  assert.strictEqual(Keymap.match('conversation', 0x49, 0, '', false), 'compose.focus');
});

test('match: a in conversation returns compose.focus', () => {
  assert.strictEqual(Keymap.match('conversation', 0x41, 0, '', false), 'compose.focus');
});

test('match: Enter in conversation returns compose.focus', () => {
  assert.strictEqual(Keymap.match('conversation', Keymap.KEY.Return, 0, '', false), 'compose.focus');
});

test('match: J (Shift+J) in conversation returns chat.next', () => {
  assert.strictEqual(Keymap.match('conversation', 0x4A, Keymap.MOD.Shift, '', false), 'chat.next');
});

test('match: K (Shift+K) in conversation returns chat.prev', () => {
  assert.strictEqual(Keymap.match('conversation', 0x4B, Keymap.MOD.Shift, '', false), 'chat.prev');
});

test('match: h in conversation returns pane.list', () => {
  assert.strictEqual(Keymap.match('conversation', 0x48, 0, '', false), 'pane.list');
});

test('match: Tab in conversation returns pane.list', () => {
  assert.strictEqual(Keymap.match('conversation', Keymap.KEY.Tab, 0, '', false), 'pane.list');
});

test('match: r in conversation returns message.retry', () => {
  assert.strictEqual(Keymap.match('conversation', 0x52, 0, '', false), 'message.retry');
});

test('match: m in conversation returns chat.mute', () => {
  assert.strictEqual(Keymap.match('conversation', 0x4D, 0, '', false), 'chat.mute');
});

test('match: u in conversation returns unread.next', () => {
  assert.strictEqual(Keymap.match('conversation', 0x55, 0, '', false), 'unread.next');
});

test('match: / in conversation returns search.focus', () => {
  assert.strictEqual(Keymap.match('conversation', 0, 0, '/', false), 'search.focus');
});

test('match: ? in conversation returns help.toggle', () => {
  assert.strictEqual(Keymap.match('conversation', 0, Keymap.MOD.Shift, '?', false), 'help.toggle');
});

test('match: q in conversation returns window.hide', () => {
  assert.strictEqual(Keymap.match('conversation', 0x51, 0, '', false), 'window.hide');
});

// Compose context.
test('match: Enter in compose returns message.send', () => {
  assert.strictEqual(Keymap.match('compose', Keymap.KEY.Return, 0, '', false), 'message.send');
});

test('match: Shift+Enter in compose returns empty (does not send)', () => {
  assert.strictEqual(Keymap.match('compose', Keymap.KEY.Return, Keymap.MOD.Shift, '', false), '');
});

// Search context.
test('match: Enter in search returns search.accept', () => {
  assert.strictEqual(Keymap.match('search', Keymap.KEY.Return, 0, '', false), 'search.accept');
});

test('match: Down in search returns search.accept', () => {
  assert.strictEqual(Keymap.match('search', Keymap.KEY.Down, 0, '', false), 'search.accept');
});

// Dialog context.
test('match: Down in dialog returns dialog.down', () => {
  assert.strictEqual(Keymap.match('dialog', Keymap.KEY.Down, 0, '', false), 'dialog.down');
});

test('match: Ctrl+J in dialog returns dialog.down', () => {
  assert.strictEqual(Keymap.match('dialog', 0x4A, Keymap.MOD.Ctrl, '', false), 'dialog.down');
});

test('match: Up in dialog returns dialog.up', () => {
  assert.strictEqual(Keymap.match('dialog', Keymap.KEY.Up, 0, '', false), 'dialog.up');
});

test('match: Ctrl+K in dialog returns dialog.up', () => {
  assert.strictEqual(Keymap.match('dialog', 0x4B, Keymap.MOD.Ctrl, '', false), 'dialog.up');
});

test('match: Enter in dialog returns dialog.accept', () => {
  assert.strictEqual(Keymap.match('dialog', Keymap.KEY.Return, 0, '', false), 'dialog.accept');
});

test('match: Ctrl+Tab in dialog returns dialog.nextAccount', () => {
  assert.strictEqual(Keymap.match('dialog', Keymap.KEY.Tab, Keymap.MOD.Ctrl, '', false), 'dialog.nextAccount');
});

// Help context.
test('match: ? in help returns help.toggle', () => {
  assert.strictEqual(Keymap.match('help', 0, Keymap.MOD.Shift, '?', false), 'help.toggle');
});

test('match: q in help returns help.close', () => {
  assert.strictEqual(Keymap.match('help', 0x51, 0, '', false), 'help.close');
});

// Shift rules: lowercase needs no Shift, uppercase needs Shift.
test('match: j without Shift works', () => {
  assert.strictEqual(Keymap.match('list', 0x4A, 0, '', false), 'cursor.down');
});

test('match: j with Shift fails', () => {
  assert.strictEqual(Keymap.match('list', 0x4A, Keymap.MOD.Shift, '', false), '');
});

test('match: G with Shift works', () => {
  assert.strictEqual(Keymap.match('list', 0x47, Keymap.MOD.Shift, '', false), 'cursor.bottom');
});

test('match: g without Shift works (matches lowercase binding)', () => {
  assert.strictEqual(Keymap.match('list', 0x47, 0, '', false), 'cursor.top');
});

// j does nothing in compose and search contexts.
test('match: j in compose context returns empty', () => {
  assert.strictEqual(Keymap.match('compose', 0x4A, 0, '', false), '');
});

test('match: j in search context returns empty', () => {
  assert.strictEqual(Keymap.match('search', 0x4A, 0, '', false), '');
});

// Punctuation specs.
test('match: / with Ctrl does not match', () => {
  assert.strictEqual(Keymap.match('list', 0, Keymap.MOD.Ctrl, '/', false), '');
});

test('match: / with Alt does not match', () => {
  assert.strictEqual(Keymap.match('list', 0, Keymap.MOD.Alt, '/', false), '');
});

test('match: / with Shift is ok (text is /)', () => {
  assert.strictEqual(Keymap.match('list', 0, Keymap.MOD.Shift, '/', false), 'search.focus');
});

test('match: ? with Shift is ok', () => {
  assert.strictEqual(Keymap.match('list', 0, Keymap.MOD.Shift, '?', false), 'help.toggle');
});

// Enter key variants.
test('match: KEY.Return matches Enter spec', () => {
  assert.strictEqual(Keymap.match('compose', Keymap.KEY.Return, 0, '', false), 'message.send');
});

test('match: KEY.Enter matches Enter spec', () => {
  assert.strictEqual(Keymap.match('compose', Keymap.KEY.Enter, 0, '', false), 'message.send');
});

// Invariant: every global binding uses Ctrl, Alt, F-key, or Escape.
test('invariant: all global bindings use safe modifiers', () => {
  const bindings = Keymap.BINDINGS.filter(function(b) { return b.contexts.indexOf('global') >= 0; });

  for (var i = 0; i < bindings.length; i++) {
    const b = bindings[i];
    for (var j = 0; j < b.keys.length; j++) {
      const spec = b.keys[j];
      const isSafe = /^(Ctrl|Alt|F\d+|Escape)/.test(spec) || spec === 'Escape';
      assert.ok(isSafe, 'Global binding ' + b.action + ' spec ' + spec + ' is not safe for typing');
    }
  }
});

// bindingsFor returns hints for a context.
test('bindingsFor returns hints for list context', () => {
  const hints = Keymap.bindingsFor('list');
  const actions = hints.map(function(h) { return h.action; });
  assert.ok(actions.includes('chat.open'), 'chat.open should be in list hints');
  assert.ok(actions.includes('search.focus'), 'search.focus should be in list hints');
  assert.ok(actions.includes('help.toggle'), 'help.toggle should be in list hints');
});

test('bindingsFor returns hints for conversation context', () => {
  const hints = Keymap.bindingsFor('conversation');
  const actions = hints.map(function(h) { return h.action; });
  assert.ok(actions.includes('compose.focus'), 'compose.focus should be in conversation hints');
  assert.ok(actions.includes('help.toggle'), 'help.toggle should be in conversation hints');
});

test('bindingsFor returns hints for compose context', () => {
  const hints = Keymap.bindingsFor('compose');
  const actions = hints.map(function(h) { return h.action; });
  assert.ok(actions.includes('message.send'), 'message.send should be in compose hints');
});

test('bindingsFor returns hints for dialog context', () => {
  const hints = Keymap.bindingsFor('dialog');
  const actions = hints.map(function(h) { return h.action; });
  assert.ok(actions.includes('dialog.accept'), 'dialog.accept should be in dialog hints');
});

// helpSections returns structured help text.
test('helpSections returns one section per context', () => {
  const sections = Keymap.helpSections();
  assert.ok(sections.length > 0, 'help sections should not be empty');

  const titles = sections.map(function(s) { return s.title; });
  assert.ok(titles.includes('Global'), 'should have Global section');
  assert.ok(titles.includes('List'), 'should have List section');
});

test('helpSections rows have keys and label', () => {
  const sections = Keymap.helpSections();
  for (var i = 0; i < sections.length; i++) {
    const section = sections[i];
    for (var j = 0; j < section.rows.length; j++) {
      const row = section.rows[j];
      assert.ok(typeof row.keys === 'string', 'row should have keys string');
      assert.ok(typeof row.label === 'string', 'row should have label string');
    }
  }
});

test('Shift+Tab arrives as Backtab and still matches Tab specs', () => {
  const { KEY, MOD } = Keymap;

  assert.strictEqual(Keymap.match('list', KEY.Backtab, MOD.Ctrl | MOD.Shift, '', false), 'rail.prev');
  assert.strictEqual(Keymap.match('list', KEY.Tab, MOD.Ctrl | MOD.Shift, '', false), 'rail.prev');
  assert.strictEqual(Keymap.match('list', KEY.Tab, MOD.Ctrl, '', false), 'rail.next');
});
