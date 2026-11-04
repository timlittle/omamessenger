.pragma library

// TypingGuard decides whether a plain key, arriving soon after the
// previous one, looks like typed text rather than a chosen shortcut.
// GUARDED_ACTIONS are held back within burstMs of any prior key; any
// action is also held back once an unmatched key has appeared earlier
// in the same burst, since that is direct evidence of typed text
// (see docs/decisions.md). Escape or any key outside this guard resets
// the tracked burst via ListController.resetBurst.

// burstMs is the gap under which a key counts as the same burst.
var burstMs = 300;

// NAVIGATION_ACTIONS are the list and conversation movement actions.
var NAVIGATION_ACTIONS = [
  'cursor.down', 'cursor.up', 'cursor.top', 'cursor.bottom',
  'message.highlightNewer', 'message.highlightOlder',
  'scroll.pageDown', 'scroll.pageUp', 'scroll.newest', 'scroll.oldest'
];

// GUARDED_ACTIONS are held back within burstMs of any previous plain key.
var GUARDED_ACTIONS = [
  'chat.archive', 'chat.archiveRead', 'chat.mute', 'chat.pin', 'chat.hide',
  'chat.block', 'chat.reportSpam', 'chat.markRead', 'chat.markUnread',
  'message.delete'
];

// next decides whether action ("" if the key matched nothing) should
// run at timestamp now, given state from the previous call ({} to
// start). Returns {allow, state} for the next call.
function next(action, now, state, navigationActions, guardedActions) {
  // A gap of burstMs or more starts a fresh burst.
  const gap = typeof state.wordAt !== 'number' || (now - state.wordAt) >= burstMs;
  const hadBefore = !gap && !!state.wordHadUnmatched;
  const hadUnmatched = hadBefore || action === '';
  const wordFields = { wordAt: now, wordHadUnmatched: hadUnmatched };

  if (navigationActions.includes(action)) {
    return { allow: !hadBefore, state: Object.assign({}, state, wordFields) };
  }

  const inBurst = typeof state.lastAt === 'number' && (now - state.lastAt) < burstMs;
  const guardedSuppressed = inBurst && guardedActions.includes(action);
  const allow = !guardedSuppressed && !hadBefore;

  return { allow: allow, state: Object.assign({ lastAt: now, lastAction: action }, wordFields) };
}

// reset clears the tracked burst state.
function reset() {
  return {};
}
