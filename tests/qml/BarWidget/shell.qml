// Checks the bar icon: the unread count (hidden at zero), the tooltip
// text, dimming while the helper is not ready, that a left click toggles
// the window through the bar's shell facade, and that settings are
// forwarded to the service on load and on every change. Uses fake
// bar/shell/service objects, not the real Service or a helper process.
import QtQuick
import QtTest
import Quickshell
import "ui"

ShellRoot {
  id: root

  property var toggleCalls: []
  property var appliedSettings: []

  // fail stops the test with a reason on stderr.
  function fail(reason: string): void {
    console.error("FAIL " + reason);
    Qt.exit(1);
  }

  // findByObjectName searches item and its descendants for a matching
  // objectName.
  function findByObjectName(item: var, name: string): var {
    if (item.objectName === name) return item;
    for (const child of item.children) {
      const found = root.findByObjectName(child, name);
      if (found) return found;
    }
    return null;
  }

  QtObject {
    id: fakeService
    property string status: "starting"
    property int unreadTotal: 0
    property bool demo: false
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
      return fail("expected settings forwarded at least once on load, got " + loadCalls);
    if (root.appliedSettings[loadCalls - 1].notifications !== true)
      return fail("settings forwarded on load were " + JSON.stringify(root.appliedSettings[loadCalls - 1]));

    if (widget.opacity >= 1)
      return fail("widget not dimmed while the helper is not ready, opacity is " + widget.opacity);

    const badge = root.findByObjectName(widget, "unreadBadge");
    if (!badge) return fail("unreadBadge not found");
    if (badge.visible) return fail("badge visible at zero unread");

    fakeService.status = "ready";
    fakeService.unreadTotal = 3;

    if (widget.opacity < 1)
      return fail("widget still dimmed once the helper is ready");
    if (!badge.visible || badge.count !== 3)
      return fail("badge did not follow unreadTotal, count=" + badge.count + " visible=" + badge.visible);
    if (widget.tooltipText !== "OmaMessenger · 3 unread")
      return fail("tooltip was '" + widget.tooltipText + "'");

    fakeService.demo = true;
    if (widget.tooltipText !== "OmaMessenger · 3 unread · demo")
      return fail("tooltip in demo mode was '" + widget.tooltipText + "'");
    fakeService.demo = false;

    fakeService.unreadTotal = 0;
    if (badge.visible) return fail("badge still visible after unreadTotal returned to zero");

    t.mouseClick(widget, widget.width / 2, widget.height / 2);
    if (root.toggleCalls.length !== 1)
      return fail("expected one toggle call, got " + root.toggleCalls.length);
    if (root.toggleCalls[0].id !== "io.github.omamessenger" || root.toggleCalls[0].payload !== "{}")
      return fail("toggle called with " + JSON.stringify(root.toggleCalls[0]));

    widget.settings = ({ notifications: false });
    if (root.appliedSettings.length !== loadCalls + 1)
      return fail("settings not forwarded exactly once more on change, got "
        + (root.appliedSettings.length - loadCalls) + " new calls");
    if (root.appliedSettings[root.appliedSettings.length - 1].notifications !== false)
      return fail("forwarded settings were " + JSON.stringify(root.appliedSettings[root.appliedSettings.length - 1]));

    console.log("PASS BarWidget");
    Qt.exit(0);
  }
}
