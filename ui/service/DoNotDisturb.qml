import QtQuick
import Quickshell.Io

// Asks Omarchy's shell, read-only, whether do-not-disturb is on right
// now, through the fixed literal argv `omarchy-shell -q notifications
// isDnd`, so this plugin's own notification sound follows the same rule.
Item {
  id: root

  // timeoutMs bounds how long check waits before giving up and
  // reporting not-on-do-not-disturb, so a sound is never held up by a slow or missing command.
  property int timeoutMs: 500

  // _callback is the one check() in flight, or null between calls.
  property var _callback: null

  // check calls callback(active) exactly once: true only for a
  // definite "on" reply within timeoutMs, false for any other outcome.
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
