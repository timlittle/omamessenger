import QtQuick
import qs.Commons
import "../theme"

// Unread count as a pill: hidden at 0, "99+" above 99. Muted conversations use
// a dimmed pill, so unread state never relies on the accent colour alone.
Item {
  id: root

  property int count: 0
  property bool muted: false
  readonly property string text: root.count > 99 ? "99+" : String(root.count)

  visible: root.count > 0
  implicitHeight: label.implicitHeight + Theme.spacing.xxs * 2
  implicitWidth: Math.max(implicitHeight, label.implicitWidth + Theme.spacing.md * 2)

  Rectangle {
    anchors.fill: parent
    radius: height / 2
    color: root.muted ? Util.alpha(Color.foreground, 0.25) : Color.accent

    Text {
      id: label
      anchors.centerIn: parent
      text: root.text
      color: root.muted ? Color.foreground : Color.background
      font.family: Theme.font.family
      font.pixelSize: Theme.font.caption
      font.weight: Font.Bold
    }
  }
}
