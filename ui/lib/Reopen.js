.pragma library

// Decides whether to reopen the window automatically once Service
// starts, after a shell restart or plugin reload (see docs/decisions.md).

// shouldReopen is true only the first time a Service asks, when the
// window was left open, the setting is on, and it hasn't reopened yet.
function shouldReopen(storedOpen, setting, alreadyReopened) {
  return !!storedOpen && !!setting && !alreadyReopened;
}
