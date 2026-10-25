.pragma library

// Decides whether to reopen the window automatically once this Service
// starts: the one case this covers is a shell restart or plugin reload
// that destroyed the window without the user asking to close it first
// (see ui/service/WindowReopen.qml and docs/decisions.md).

// shouldReopen is true only the first time a Service asks: the window
// must have been left open rather than deliberately closed
// (storedOpen), the "Reopen after restart" setting must be on
// (setting), and this Service must not already have reopened it once
// (alreadyReopened). A fresh install, where storedOpen is always
// false, never reopens.
function shouldReopen(storedOpen, setting, alreadyReopened) {
  return !!storedOpen && !!setting && !alreadyReopened;
}
