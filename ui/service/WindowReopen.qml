import QtQuick
import "../lib/Reopen.js" as Reopen

// Reopens the window once, the first time this Service starts, when the
// helper's own store says it was last left open rather than deliberately
// closed - the one run after a shell restart or plugin reload destroyed
// it without the user asking to close it (see docs/decisions.md). A
// fresh install, where nothing has ever been recorded, never auto-opens.
Item {
  id: root

  // request runs a helper method; the same signature Service.request
  // already has, passed straight through rather than a whole Service
  // reference so this stays easy to drive with a fake in tests.
  property var request: null

  // shell is the host facade used to summon the window back open.
  property var shell: null

  // reopenAfterRestart is the "Reopen after restart" setting; off skips
  // the check entirely.
  property bool reopenAfterRestart: true

  // reopened is true once this Service has summoned the window back,
  // so a later helper reconnect within the same run never does it
  // again; also read by tests to confirm it happens at most once.
  property bool reopened: false

  // _asked guards asking the helper more than once per Service start: a
  // helper reconnect calls check() again before the first request's
  // reply has even come back.
  property bool _asked: false

  // check asks the helper whether the window was last left open and
  // reopens it if so. A no-op after the first call in this Service's
  // lifetime.
  function check(): void {
    if (root._asked) return;
    root._asked = true;
    if (typeof root.request !== "function") return;

    root.request("ui.getWindowOpen", {}, function(error, result) {
      if (error) return;
      if (!Reopen.shouldReopen(result.open, root.reopenAfterRestart, root.reopened)) return;

      root.reopened = true;
      root._summon();
    });
  }

  // _summon reopens the panel the same way a clicked notification does:
  // through the shell facade's summon, never toggle, so it can only
  // ever open the window, not accidentally close one already open.
  function _summon(): void {
    if (!root.shell || typeof root.shell.summon !== "function") return;

    root.shell.summon("io.github.omamessenger", "{}");
  }
}
