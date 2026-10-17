.pragma library

// Decides whether a click to open a video or file's media should
// actually hand it to the user's application, or be ignored as a
// repeat click still within its cooldown. Nothing tells MediaController
// when, or whether, the external player it handed a file to ever
// closes, so time since that message was last opened is the only
// signal left to tell a repeat click from a deliberate new one.

// canOpen reports whether the message that was last opened at
// lastOpenedAt (its entry in MediaController's per-message map, or
// undefined/null when it has never been opened) may be opened again
// now. True when it has never been opened, or its last open was at
// least cooldownMs ago.
function canOpen(lastOpenedAt, now, cooldownMs) {
  if (lastOpenedAt === undefined || lastOpenedAt === null) {
    return true;
  }

  return now - lastOpenedAt >= cooldownMs;
}
