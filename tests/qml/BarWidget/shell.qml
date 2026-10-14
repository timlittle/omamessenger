// Checks the bar icon: the unread count (hidden at zero), the tooltip
// text and that it reaches the bar's own tooltip bubble on hover and
// leaves it on exit, dimming while the helper is not ready, that a left
// click toggles the window through the bar's shell facade, and that
// settings are forwarded to the service on load and on every change.
// Uses fake bar/shell/service objects, not the real Service or a helper
// process.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property var toggleCalls: []
  property var appliedSettings: []
  property var tooltipCalls: []
  property int hideTooltipCalls: 0

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
    function showTooltip(target, text) { root.tooltipCalls.push({ target: target, text: text }); }
    function hideTooltip(target) { root.hideTooltipCalls += 1; }
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

    // Hovering asks the bar to show its own tooltip bubble with the
    // widget's current text; the formatting itself is Tooltip.js's job,
    // proven exhaustively by its own node tests (see tests/unit).
    t.mouseMove(widget, widget.width / 2, widget.height / 2);
    if (root.tooltipCalls.length !== 1)
      return Check.fail("expected one showTooltip call on hover, got " + root.tooltipCalls.length);
    if (root.tooltipCalls[0].target !== widget || root.tooltipCalls[0].text !== widget.tooltipText)
      return Check.fail("showTooltip called with " + JSON.stringify(root.tooltipCalls[0]));

    t.mouseMove(widget, -10, -10);
    if (root.hideTooltipCalls !== 1)
      return Check.fail("expected one hideTooltip call on leaving the widget, got " + root.hideTooltipCalls);

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
