import QtQuick
import "../lib/Reopen.js" as Reopen

// Reopens the window once at Service start if the helper's store says
// it was last left open rather than closed deliberately (see
// docs/decisions.md).
Item {
  id: root

  // request runs a helper method; same signature as Service.request.
  property var request: null

  // shell is the host facade used to summon the window back open.
  property var shell: null

  // reopenAfterRestart is the "Reopen after restart" setting.
  property bool reopenAfterRestart: true

  // reopened is true once the window has been summoned back this run.
  property bool reopened: false

  // _asked guards against asking the helper more than once per run.
  property bool _asked: false

  // check asks the helper whether the window was last open and reopens
  // it if so; a no-op after the first call.
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

  // _summon opens the panel via summon, never toggle, so it can only
  // open the window, never close one already open.
  function _summon(): void {
    if (!root.shell || typeof root.shell.summon !== "function") return;

    root.shell.summon("io.github.omamessenger", "{}");
  }
}
