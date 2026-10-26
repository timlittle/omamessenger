.pragma library

// TypingGuard decides whether a plain key, arriving right on the heels
// of the one before it in the list or conversation context, looks like
// someone typing into the wrong place rather than reaching for a
// shortcut. Only GUARDED_ACTIONS - the ones that change state with the
// service or lose something, and so are worth delaying by one keypress
// - are ever held back; a mode switch (writing, replying, editing),
// opening something, or moving the highlight always runs regardless of
// timing, since none of those loses anything on its own. An unmatched
// key (no action at all, the shape an ordinary typed word takes) still
// counts as part of a burst, since that is the actual "typing in the
// wrong place" signal this exists to catch.

// burstMs is the gap, in milliseconds, under which a key counts as the
// same burst as the one before it.
var burstMs = 300;

// NAVIGATION_ACTIONS are the list and conversation context's own
// movement actions (j/k/g/G/Home/End/PageUp/PageDown and the arrows
// each of those stand in for): exempt both ways, so scrolling or
// moving the highlight quickly, then reaching for a shortcut right
// afterwards, is never mistaken for a burst.
var NAVIGATION_ACTIONS = [
  'cursor.down', 'cursor.up', 'cursor.top', 'cursor.bottom',
  'message.highlightNewer', 'message.highlightOlder',
  'scroll.pageDown', 'scroll.pageUp', 'scroll.newest', 'scroll.oldest'
];

// GUARDED_ACTIONS are the only actions next ever holds back: each one
// archives, mutes, pins, hides, blocks, reports, deletes or marks
// read/unread - the conversation-changing actions ui/lib/Undo.js knows
// how to reverse, plus mark read/unread, which it does not (see
// docs/decisions.md). Every other action - compose.focus, message.edit,
// message.reply, message.react, chat.open, escape, search and palette
// openers - always runs, however fast it follows another key.
var GUARDED_ACTIONS = [
  'chat.archive', 'chat.archiveRead', 'chat.mute', 'chat.pin', 'chat.hide',
  'chat.block', 'chat.reportSpam', 'chat.markRead', 'chat.markUnread',
  'message.delete'
];

// next decides whether action (the one a plain key just matched, or ""
// when it matched nothing) should run at timestamp now: state is
// whatever the previous call returned ({} to start). Only an action in
// guardedActions can ever be held back, and only once a previous
// plain key - matched or not - landed within burstMs of it; everything
// else always runs, but still updates the returned state, so a run of
// ordinary letters that match nothing still builds towards holding
// back a guarded action the moment one is actually pressed. Navigation
// is exempt from all of this: it is left out of the returned state
// entirely, so it can neither start a burst nor inherit one already in
// progress.
function next(action, now, state, navigationActions, guardedActions) {
  if (navigationActions.includes(action)) {
    return { allow: true, state: state };
  }

  const inBurst = typeof state.lastAt === 'number' && (now - state.lastAt) < burstMs;
  const allow = !(inBurst && guardedActions.includes(action));

  return { allow: allow, state: { lastAt: now, lastAction: action } };
}
