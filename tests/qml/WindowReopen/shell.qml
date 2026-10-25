// Checks the "reopen after restart" feature from two angles. First,
// ui/service/WindowReopen.qml's own wiring into ui/lib/Reopen.js's
// shouldReopen (itself proven exhaustively in
// tests/unit/reopen.test.cjs): a fake request/shell, no helper needed,
// confirms a fresh install never summons the window, a stored open
// state summons it exactly once even across a second check() call (as a
// helper reconnect would make), and the "Reopen after restart" setting
// off prevents it. Second, ui/Panel.qml's own recording of
// markWindowOpen/markWindowClosed against the real helper's ui_state
// store, through a real Service and FakeShell: opening records open,
// the close question's "Keep in background" and a bar icon click that
// closes it both record closed, while the host calling close() itself
// (as it does for every panel when reloading plugins) and tearing the
// panel down without close() never record closed.
import QtQuick
import Quickshell
import "ui"
import "ui/service"
import "Check.js" as Check

ShellRoot {
  id: root

  property var steps: [
    root.waitForReady,
    root.openPanel,
    root.pollStoredOpenTrue,
    root.askToCloseAndKeep,
    root.pollStoredOpenFalse,
    root.reopenPanel,
    root.pollStoredOpenTrue,
    root.hostClose,
    root.pollStoredOpenStillTrue,
    root.reopenPanelAgain,
    root.pollStoredOpenTrue,
    root.barClose,
    root.pollStoredOpenFalse,
    root.reopenPanelAgain,
    root.pollStoredOpenTrue,
    root.teardownPanel,
    root.pollStoredOpenStillTrue
  ]

  // _lastStoredOpen/_lastStoredOpenKnown cache the last ui.getWindowOpen
  // reply, read by pollStoredOpen; _refreshStoredOpen asks again.
  property bool _lastStoredOpen: false
  property bool _lastStoredOpenKnown: false

  function _refreshStoredOpen(): void {
    helperService.request("ui.getWindowOpen", {}, function(error, result) {
      if (error) return;
      root._lastStoredOpen = result.open;
      root._lastStoredOpenKnown = true;
    });
  }

  // pollStoredOpen is the shared poll body: true once a reply matching
  // expected has arrived, otherwise it asks again and waits.
  function pollStoredOpen(expected: bool): var {
    if (root._lastStoredOpenKnown && root._lastStoredOpen === expected) return true;
    root._refreshStoredOpen();
    return false;
  }

  function pollStoredOpenTrue(): var { return root.pollStoredOpen(true); }
  function pollStoredOpenFalse(): var { return root.pollStoredOpen(false); }
  function pollStoredOpenStillTrue(): var { return root.pollStoredOpen(true); }

  // panel is the live Panel; windowController the real controller the
  // close question runs through.
  function panel(): var { return panelLoader.item; }
  function windowController(): var { return Check.find(root.panel(), "windowController"); }

  function waitForReady(): var {
    return helperService.status === "ready";
  }

  // openPanel shows the window, which ui/Panel.qml's open() records as
  // open through service.markWindowOpen().
  function openPanel(): var {
    root._lastStoredOpenKnown = false;
    root.panel().open("{}");
    return true;
  }

  function reopenPanel(): var { return root.openPanel(); }
  function reopenPanelAgain(): var { return root.openPanel(); }

  // askToCloseAndKeep answers the close question with "Keep in
  // background", the same choice a real Enter press on its safe default
  // makes: a deliberate close that must record the window as closed.
  function askToCloseAndKeep(): var {
    root._lastStoredOpenKnown = false;
    const wc = root.windowController();
    wc.askToClose();
    wc.keepInBackground();
    return true;
  }

  // hostClose calls the panel's own close() directly, the way the host
  // does for every panel while reloading plugins: that says nothing
  // about whether the user meant to close it, so it must leave the
  // window recorded as open.
  function hostClose(): var {
    root._lastStoredOpenKnown = false;
    root.panel().close();
    return true;
  }

  // barClose clicks the bar icon while the window is showing, through
  // the same Service.toggleFromBar the bar widget uses: a deliberate
  // close, which must record the window as closed. The shell here only
  // records the toggle, so it never reopens the window behind the test.
  function barClose(): var {
    root._lastStoredOpenKnown = false;
    helperService.toggleFromBar({ toggle: function(id, payload) {} }, "io.github.omamessenger");
    return true;
  }

  // teardownPanel destroys the panel without calling close() first, the
  // way Omarchy tears it down for a shell restart or plugin reload:
  // this must never record the window as closed.
  function teardownPanel(): var {
    root._lastStoredOpenKnown = false;
    panelLoader.active = false;
    return true;
  }

  // freshInstallSummons counts summon calls a fresh install must never
  // make.
  property int freshInstallSummons: 0

  WindowReopen {
    id: freshInstallReopen
    reopenAfterRestart: true
    request: function(method, params, callback) {
      if (method === "ui.getWindowOpen") callback(null, { open: false });
    }
    shell: ({ summon: function(id, payload) { root.freshInstallSummons++; } })
  }

  // storedOpenSummons records every summon call storedOpenReopen makes,
  // so the test can confirm there is exactly one.
  property var storedOpenSummons: []

  WindowReopen {
    id: storedOpenReopen
    reopenAfterRestart: true
    request: function(method, params, callback) {
      if (method === "ui.getWindowOpen") callback(null, { open: true });
    }
    shell: ({ summon: function(id, payload) { root.storedOpenSummons.push({ id: id, payload: payload }); } })
  }

  // settingOffSummons counts summon calls settingOffReopen must never
  // make, since its own reopenAfterRestart is false.
  property int settingOffSummons: 0

  WindowReopen {
    id: settingOffReopen
    reopenAfterRestart: false
    request: function(method, params, callback) {
      if (method === "ui.getWindowOpen") callback(null, { open: true });
    }
    shell: ({ summon: function(id, payload) { root.settingOffSummons++; } })
  }

  Service {
    id: helperService
  }

  FakeShell {
    id: fakeShell
    panel: root.panel()
    service: helperService
  }

  Loader {
    id: panelLoader

    sourceComponent: Panel {
      service: helperService
      shell: fakeShell
    }
  }

  Stepper {
    id: stepper
    name: "WindowReopen"
    steps: root.steps
    deadlineMs: 30000
    onTimeout: () => Check.fail("timed out before the checks finished")
    startFn: root.checkWindowReopenWiring
  }

  // checkWindowReopenWiring runs the three fake request/shell checks
  // synchronously, before the step chain against the real helper
  // starts: none of them need a helper round trip, since the fake
  // request's callback runs straight away.
  function checkWindowReopenWiring(): void {
    freshInstallReopen.check();
    if (root.freshInstallSummons !== 0)
      return Check.fail("a fresh install summoned the window " + root.freshInstallSummons + " times, want 0");

    storedOpenReopen.check();
    storedOpenReopen.check(); // a second call, as a helper reconnect would make, must not summon again
    if (root.storedOpenSummons.length !== 1)
      return Check.fail("a stored open state summoned the window " + root.storedOpenSummons.length + " times, want 1");
    if (root.storedOpenSummons[0].id !== "io.github.omamessenger")
      return Check.fail("summoned the wrong plugin id: " + JSON.stringify(root.storedOpenSummons[0]));

    settingOffReopen.check();
    if (root.settingOffSummons !== 0)
      return Check.fail("the \"Reopen after restart\" setting being off still summoned the window");

    stepper.runStep();
  }
}
