pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import "../theme"
import "../lib/Toast.js" as ToastLogic

// A small, rounded confirmation pill: where copying, saving and the
// "Updated to vX" hint now show, instead of the easily-missed banner at
// the top of the window (see docs/decisions.md). It fades in whenever
// text is set, auto-dismisses itself after dismissDelayMs, and pauses
// that countdown while the pointer is over it so reading it does not
// race its own timer. Errors keep using Panel's own banner; this is
// only ever good news, so it never needs the urgent colour.
Item {
  id: root

  // text is the notice to show, or "" to hide the toast entirely; the
  // caller (WindowController.toastText) already picked which of the
  // update hint or an ordinary notice currently wins.
  property string text: ""
  // dismissDelayMs is how long the toast stays up, once shown, before
  // it dismisses itself; a property rather than a literal so a test can
  // shrink the wait instead of sitting through the real few seconds.
  property int dismissDelayMs: 3000

  // _hovered pauses the auto-dismiss countdown while the pointer sits
  // over the pill.
  property bool _hovered: false

  // _label is text paired with a glyph, so the pill is never read by
  // colour alone.
  readonly property string _label: ToastLogic.displayText(root.text)

  // dismissed fires when the toast should be cleared: its own timer
  // once it is not paused, or a click anywhere on the pill.
  signal dismissed()

  objectName: "toast"
  visible: root._label !== ""
  implicitWidth: Math.min(pill.implicitWidth, Style.space(420))
  implicitHeight: pill.implicitHeight
  opacity: root._label !== "" ? 1 : 0

  Behavior on opacity {
    NumberAnimation { duration: 120 }
  }

  // dismissTimer restarts from dismissDelayMs every time the shown text
  // changes (a new notice always gets its own full look, the newest
  // simply replacing whatever was showing rather than queuing behind
  // it) and stops while the pointer hovers the pill, resuming at the
  // full delay once it leaves. running is driven entirely by the calls
  // below rather than a binding, since restarting a Timer whose
  // running is itself bound would silently break that binding.
  Timer {
    id: dismissTimer
    interval: root.dismissDelayMs
    onTriggered: root.dismissed()
  }

  onTextChanged: {
    if (root.text.trim() === "") { dismissTimer.stop(); return; }
    if (!root._hovered) dismissTimer.restart();
  }

  Component.onCompleted: {
    if (root.text.trim() !== "") dismissTimer.restart();
  }

  Rectangle {
    id: pill
    objectName: "toastPill"
    anchors.fill: parent
    radius: height / 2
    color: Theme.popups.background
    border.color: Theme.popups.border
    border.width: Style.normalBorderWidth
    implicitWidth: label.implicitWidth + Theme.spacing.lg * 2
    implicitHeight: label.implicitHeight + Theme.spacing.sm * 2

    Text {
      id: label
      objectName: "toastLabel"
      anchors.centerIn: parent
      text: root._label
      // A notice is always this plugin's own words, never a service's,
      // but PlainText is free insurance against auto-detected rich
      // text the same way every other label in this window keeps it.
      textFormat: Text.PlainText
      color: Theme.popups.text
      font.family: Theme.font.family
      font.pixelSize: Theme.font.bodySmall
    }

    MouseArea {
      objectName: "toastDismissArea"
      anchors.fill: parent
      hoverEnabled: true
      cursorShape: Qt.PointingHandCursor
      onEntered: { root._hovered = true; dismissTimer.stop(); }
      onExited: {
        root._hovered = false;
        if (root.text.trim() !== "") dismissTimer.restart();
      }
      onClicked: root.dismissed()
    }
  }
}
