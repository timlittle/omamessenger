.pragma library

// TypingGuard decides whether a burst of plain key presses in the list
// or conversation context looks like someone typing into the wrong
// place rather than reaching for a shortcut: a second plain key
// arriving right on the heels of the one before it. Navigation
// (scrolling, moving the highlight) is exempt: it is meant to repeat
// quickly and never counts towards, or is itself stopped by, a burst.

// burstMs is the gap, in milliseconds, under which a key counts as the
// same burst as the one before it.
var burstMs = 300;

// NAVIGATION_ACTIONS are the list and conversation context's own
// movement actions (j/k/g/G/Home/End/PageUp/PageDown and the arrows
// each of those stand in for): the ones a key router must never
// suppress, and which never start or extend a burst themselves, so
// scrolling or moving the highlight quickly, then reaching for a
// shortcut right afterwards, is never mistaken for one.
var NAVIGATION_ACTIONS = [
  'cursor.down', 'cursor.up', 'cursor.top', 'cursor.bottom',
  'message.highlightNewer', 'message.highlightOlder',
  'scroll.pageDown', 'scroll.pageUp', 'scroll.newest', 'scroll.oldest'
];

// next decides whether action, matched at timestamp now, should run:
// state is whatever the previous call returned ({} to start), and
// navigationActions is the list of actions next never suppresses or
// tracks (pass TypingGuard.NAVIGATION_ACTIONS). Returns {allow, state}:
// state carries lastAt/lastAction forward for the following call, and
// is left unchanged by a navigation action, so one does not itself
// start a new burst or inherit an old one.
function next(action, now, state, navigationActions) {
  if (navigationActions.includes(action)) {
    return { allow: true, state: state };
  }

  const burst = !!state.lastAction && (now - state.lastAt) < burstMs;

  return { allow: !burst, state: { lastAt: now, lastAction: action } };
}
