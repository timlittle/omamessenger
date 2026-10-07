import QtQuick
import Quickshell.Io

// Runs the plugin's helper launcher as a child process and restarts it on
// crash. status is one of "starting", "ready", "stopped", "error" or
// "missing": starting while the process is launching, ready once
// Quickshell reports it running and able to accept JSON-RPC requests,
// stopped after a deliberate stop, error once the restart budget below is
// exhausted, and missing when the launcher's exit code says no helper is
// installed.
//
// Item rather than QtObject: only a type with a default property can hold
// the Process and Timer children below without naming a property for them.
Item {
  id: root


  // status reports the helper's lifecycle; see the type comment above.
  property string status: "starting"

  // detail is the helper's last line of stderr, shown beside the status.
  property string detail: ""

  // line is emitted once per line the helper writes to stdout: one
  // JSON-RPC response or notification.
  signal line(string text)

  // _launcherPath is bin/oma-messenger-service, resolved from this file's
  // own location so a dev checkout and an installed, symlinked copy both
  // find the same launcher.
  readonly property string _launcherPath: String(Qt.resolvedUrl("../../bin/oma-messenger-service")).replace("file://", "")

  // _installScriptPath is the script install() runs.
  readonly property string _installScriptPath: String(Qt.resolvedUrl("../../scripts/install-helper.sh")).replace("file://", "")

  // _exitTimes holds the timestamps of recent crashes, pruned to the last
  // 60 seconds, so five crashes in that window stop the restarts.
  property var _exitTimes: []

  // _stopping is true only during the deliberate stop() path, so the exit
  // handler does not treat that exit as a crash.
  property bool _stopping: false

  // _startAfterStop asks the exit handler to start the helper again, for a
  // start() that arrived while a stopped helper was still exiting.
  property bool _startAfterStop: false

  // start launches the helper. If one is still shutting down after
  // stop(), it starts again once that one has exited.
  function start(): void {
    if (helper.running) {
      root._startAfterStop = root._stopping;
      return;
    }

    console.info("OmaMessenger: starting the helper");
    root.status = "starting";
    root.detail = "";
    helper.command = [root._launcherPath];
    helper.running = true;
  }

  // stop ends the helper deliberately; the exit handler will not restart it.
  function stop(): void {
    console.info("OmaMessenger: stopping the helper");
    root._stopping = true;
    helper.running = false;
    root.status = "stopped";
  }

  // write sends one line to the helper's stdin.
  function write(text: string): void {
    helper.write(text);
  }

  // install runs the install script and starts the helper once it reports
  // success.
  function install(): void {
    installer.running = true;
  }

  // _handleExit applies the restart policy to one helper exit.
  function _handleExit(exitCode: int): void {
    console.info(`OmaMessenger: the helper exited with code ${exitCode}${root._stopping ? " when asked to stop" : ""}`);
    if (root._stopping) {
      root._stopping = false;
      if (root._startAfterStop) {
        root._startAfterStop = false;
        root.start();
      }
      return;
    }

    if (exitCode === 3) {
      root.status = "missing";
      return;
    }

    root._scheduleRestart();
  }

  // _scheduleRestart records this crash and either retries after 1, 3 or
  // 10 seconds, or gives up once five crashes land inside 60 seconds.
  function _scheduleRestart(): void {
    const now = Date.now();
    root._exitTimes = root._exitTimes.filter((t) => now - t < 60000);
    root._exitTimes.push(now);

    if (root._exitTimes.length >= 5) {
      root.status = "error";
      return;
    }

    const delays = [1000, 3000, 10000];
    restartTimer.interval = delays[Math.min(root._exitTimes.length - 1, delays.length - 1)];
    restartTimer.start();
  }

  Timer {
    id: restartTimer
    repeat: false
    onTriggered: root.start()
  }

  Process {
    id: helper
    stdinEnabled: true

    stdout: SplitParser {
      onRead: function(data) { root.line(data); }
    }

    // The helper's diagnostics never hold message content (see
    // tools/nologcontent), so they go to the shell's log as well.
    stderr: SplitParser {
      onRead: function(data) {
        root.detail = data;
        console.info(`OmaMessenger helper: ${data}`);
      }
    }

    onRunningChanged: {
      if (helper.running) root.status = "ready";
    }
  }

  // Binding exited through Connections, rather than the Process's own
  // onExited, sidesteps a Quickshell metadata gap: its exitStatus
  // parameter type is not registered for qmllint to compile a direct
  // handler against.
  Connections {
    target: helper
    function onExited(exitCode) { root._handleExit(exitCode); }
  }

  Process {
    id: installer
    command: [root._installScriptPath]

    stderr: SplitParser {
      onRead: function(data) { root.detail = data; }
    }
  }

  Connections {
    target: installer
    function onExited(exitCode) {
      if (exitCode === 0) root.start();
    }
  }

  Component.onCompleted: root.start()
  Component.onDestruction: console.info("OmaMessenger: the helper's service was destroyed")
}
