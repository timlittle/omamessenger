import QtQuick
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// Unread count as a pill: hidden at 0, "99+" above 99. Muted conversations use
// a dimmed pill, so unread state never relies on the accent colour alone.
Item {
  id: root

  // count is the number of unread messages the pill names.
  property int count: 0
  // muted dims the pill so a muted conversation stays quiet in the list.
  property bool muted: false
  // text is what the pill shows: the count, capped at "99+" so it never
  // grows wide enough to crowd whatever sits beside it.
  readonly property string text: Format.unreadLabel(root.count)

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
