import QtQuick
import Quickshell.Io

// Asks Omarchy's own shell, read-only, whether its own do-not-disturb
// is on right now, through the same fixed, literal argv
// `omarchy-shell -q notifications isDnd` our own rules already call
// safe for a read-only query (see omarchy.md's "listPlugins" note):
// never a string built from anything this helper or a sender
// controls. Omarchy's own notification daemon already silences a
// popup under do-not-disturb; this is only so this plugin's own
// notification sound follows the same rule, rather than sounding
// through it.
Item {
  id: root

  // timeoutMs bounds how long check waits for an answer before giving
  // up and reporting not-on-do-not-disturb: a notification sound must
  // never be held up for long by an external command that is slow,
  // missing, or not running inside Omarchy at all (a dev build, a
  // test).
  property int timeoutMs: 500

  // _callback is the one check() in flight, or null between calls.
  property var _callback: null

  // check calls callback(active) exactly once: true only for a
  // definite "on" reply within timeoutMs, false for anything else -
  // "off", a missing shell, an unreadable reply, a check already in
  // flight, or one that simply never answers in time. A notification
  // sound is never worth blocking on or failing over, so every one of
  // those failure shapes resolves the same safe way: not on
  // do-not-disturb, play as usual.
  function check(callback: var): void {
    if (root._callback) { callback(false); return; }

    root._callback = callback;
    process.running = true;
    timeoutTimer.restart();
  }

  // _finish answers the in-flight check, once, then clears it; a
  // second call (the process both printing a line and then exiting,
  // or the timeout firing after an answer already arrived) is a no-op.
  function _finish(active: bool): void {
    if (!root._callback) return;

    timeoutTimer.stop();
    const callback = root._callback;
    root._callback = null;
    callback(active);
  }

  Process {
    id: process
    command: ["omarchy-shell", "-q", "notifications", "isDnd"]

    stdout: SplitParser {
      onRead: function(data) { root._finish(data.trim() === "on"); }
    }
  }

  // Connections, rather than Process's own onExited, sidesteps the
  // same qmllint metadata gap HelperProcess.qml's own comment explains:
  // exitStatus's type is not registered for a direct handler to compile.
  Connections {
    target: process
    function onExited(exitCode) { root._finish(false); }
  }

  Timer {
    id: timeoutTimer
    interval: root.timeoutMs
    onTriggered: {
      process.running = false;
      root._finish(false);
    }
  }
}
