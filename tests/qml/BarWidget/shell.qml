// Checks the bar icon: the unread count (hidden at zero), the tooltip
// text, dimming while the helper is not ready, that a left click toggles
// the window through the bar's shell facade, and that settings are
// forwarded to the service on load and on every change. Uses fake
// bar/shell/service objects, not the real Service or a helper process.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property var toggleCalls: []
  property var appliedSettings: []

  QtObject {
    id: fakeService
    property string status: "starting"
    property int unreadTotal: 0
    function applySettings(settings) { root.appliedSettings.push(settings); }
  }

  QtObject {
    id: fakeShell
    function serviceFor(id) { return fakeService; }
    function toggle(id, payloadJson) { root.toggleCalls.push({ id: id, payload: payloadJson }); }
  }

  QtObject {
    id: fakeBar
    property QtObject shell: fakeShell
  }

  FloatingWindow {
    id: win
    implicitWidth: 60
    implicitHeight: 60
    visible: true

    BarWidget {
      id: widget
      anchors.centerIn: parent
      bar: fakeBar
      settings: ({ notifications: true })
    }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    running: true
    interval: 50
    onTriggered: root.run()
  }

  // run drives the widget and checks every behaviour in turn.
  function run(): void {
    const loadCalls = root.appliedSettings.length;
    if (loadCalls < 1)
      return Check.fail("expected settings forwarded at least once on load, got " + loadCalls);
    if (root.appliedSettings[loadCalls - 1].notifications !== true)
      return Check.fail("settings forwarded on load were " + JSON.stringify(root.appliedSettings[loadCalls - 1]));

    if (widget.opacity >= 1)
      return Check.fail("widget not dimmed while the helper is not ready, opacity is " + widget.opacity);

    const badge = Check.find(widget, "unreadBadge");
    if (!badge) return Check.fail("unreadBadge not found");
    if (badge.visible) return Check.fail("badge visible at zero unread");

    fakeService.status = "ready";
    fakeService.unreadTotal = 3;

    if (widget.opacity < 1)
      return Check.fail("widget still dimmed once the helper is ready");
    if (!badge.visible || badge.count !== 3)
      return Check.fail("badge did not follow unreadTotal, count=" + badge.count + " visible=" + badge.visible);
    if (widget.tooltipText !== "OmaMessenger · 3 unread")
      return Check.fail("tooltip was '" + widget.tooltipText + "'");

    fakeService.unreadTotal = 0;
    if (badge.visible) return Check.fail("badge still visible after unreadTotal returned to zero");

    t.mouseClick(widget, widget.width / 2, widget.height / 2);
    if (root.toggleCalls.length !== 1)
      return Check.fail("expected one toggle call, got " + root.toggleCalls.length);
    if (root.toggleCalls[0].id !== "io.github.omamessenger" || root.toggleCalls[0].payload !== "{}")
      return Check.fail("toggle called with " + JSON.stringify(root.toggleCalls[0]));

    widget.settings = ({ notifications: false });
    if (root.appliedSettings.length !== loadCalls + 1)
      return Check.fail("settings not forwarded exactly once more on change, got "
        + (root.appliedSettings.length - loadCalls) + " new calls");
    if (root.appliedSettings[root.appliedSettings.length - 1].notifications !== false)
      return Check.fail("forwarded settings were " + JSON.stringify(root.appliedSettings[root.appliedSettings.length - 1]));

    console.log("PASS BarWidget");
    Qt.exit(0);
  }
}
