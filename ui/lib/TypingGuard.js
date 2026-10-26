.pragma library

// TypingGuard decides whether a plain key, arriving right on the heels
// of the one before it in the list or conversation context, looks like
// someone typing into the wrong place rather than reaching for a
// shortcut. Two independent checks feed the one answer next() returns:
//
// - GUARDED_ACTIONS (archive, mute, pin, hide, block, delete, mark
//   read/unread - the conversation-changing actions ui/lib/Undo.js
//   knows how to reverse, plus mark read/unread, which it does not) are
//   held back the moment any plain key, matched or not, landed within
//   burstMs before them. This is the original, narrower rule: it never
//   holds back a mode switch (writing, replying, editing), an opener or
//   navigation, since none of those loses anything on its own.
//
// - Every printable-key action - GUARDED_ACTIONS and navigation
//   included - is also held back once an unmatched key (action === "",
//   the shape an ordinary typed letter takes once it lands on one not
//   bound to anything) has actually been seen earlier in the same
//   burst: direct evidence that this is a typed word, not a chosen
//   shortcut, as 'a' is in "jam" or 'h' is in "hello" (see
//   docs/decisions.md). This catches a report's "hello" opening the
//   selected chat on its third letter and leaking the rest into the
//   composer - chat.open and compose.focus are never in
//   GUARDED_ACTIONS, since opening or writing loses nothing on their
//   own, but running three letters into either one anyway is exactly
//   the mistake this exists to catch.
//
//   A burst made up only of real, matched actions - however many, and
//   however fast one follows another - is never held back by this
//   second check on its own: three deliberate shortcuts chained
//   quickly (archive, archive-and-mark-read, then scroll, say) must
//   always still work, the same as two (Shift+E then i, r then i).
//   There is no way to tell that apart from someone typing a word that
//   happens to consist entirely of letters this context has bound to
//   something, with timing alone; an unmatched key is the one
//   unambiguous signal this guard has, so that is the one it uses.
//
// Both checks reset the moment Escape, or any other key this guard is
// never even called for (see Panel.routeKey's own isPlainKey check),
// arrives: ListController.resetBurst clears the tracked state then.

// burstMs is the gap, in milliseconds, under which a key counts as the
// same burst as the one before it.
var burstMs = 300;

// NAVIGATION_ACTIONS are the list and conversation context's own
// movement actions (j/k/g/G/Home/End/PageUp/PageDown and the arrows
// each of those stand in for).
var NAVIGATION_ACTIONS = [
  'cursor.down', 'cursor.up', 'cursor.top', 'cursor.bottom',
  'message.highlightNewer', 'message.highlightOlder',
  'scroll.pageDown', 'scroll.pageUp', 'scroll.newest', 'scroll.oldest'
];

// GUARDED_ACTIONS are the actions the first check above ever holds
// back on their own, within burstMs of any previous plain key.
var GUARDED_ACTIONS = [
  'chat.archive', 'chat.archiveRead', 'chat.mute', 'chat.pin', 'chat.hide',
  'chat.block', 'chat.reportSpam', 'chat.markRead', 'chat.markUnread',
  'message.delete'
];

// next decides whether action (the one a plain key just matched, or ""
// when it matched nothing) should run at timestamp now: state is
// whatever the previous call returned ({} to start). Returns {allow,
// state}, the state to pass into the following call.
function next(action, now, state, navigationActions, guardedActions) {
  // hadUnmatched carries forward whether an unmatched key has already
  // landed earlier in this same burst (see the file comment); a gap of
  // burstMs or more starts a fresh one. It is the one thing this file
  // tracks that every action, navigation included, is judged against.
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

// reset clears every burst this file tracks, for a key - Escape, or
// any other this guard is never called for - that should never carry
// a burst across it; see ListController.resetBurst, the one caller.
function reset() {
  return {};
}
