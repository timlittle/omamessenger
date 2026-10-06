import QtQuick
import qs.Commons

// Unread message count badge.
//
// Displays a circular badge with the message count. Hidden when count is 0.
// Shows "99+" for counts above 99. When muted, uses a muted background color.
Item {
  id: root

  // ------------------------------------------------------------- API
  property int count: 0
  property bool muted: false
  readonly property string text: root.count > 99 ? "99+" : root.count.toString()

  // ------------------------------------------------------------- impl
  implicitWidth: badge.implicitWidth
  implicitHeight: badge.implicitHeight
  visible: root.count > 0

  Rectangle {
    id: badge
    width: Math.max(implicitWidth, implicitHeight)
    height: width
    radius: width / 2

    color: root.muted ? Util.alpha(Color.foreground, 0.25) : Color.accent

    implicitWidth: Math.max(24, contentText.implicitWidth + 8)
    implicitHeight: 24

    Text {
      id: contentText
      anchors.centerIn: parent
      text: root.text
      color: root.muted ? Color.foreground : Color.background
      font.pixelSize: 10
      font.weight: Font.Black
      horizontalAlignment: Text.AlignHCenter
      verticalAlignment: Text.AlignVCenter
    }
  }
}
