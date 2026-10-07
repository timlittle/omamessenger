import QtQuick
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// A contact's initials in a quiet circle, optionally tinted so a group
// chat's senders stay visually distinct from one another.
Item {
  id: root

  property string name: ""
  property int size: Style.space(32)
  // tint is the circle's colour when tinted is true, typically a
  // sender's own colour in a group chat; ignored otherwise, so every
  // other caller keeps the original neutral look.
  property color tint: Color.foreground
  // tinted switches the circle from its default neutral fill to tint.
  property bool tinted: false
  readonly property string text: Format.initials(root.name)

  implicitWidth: root.size
  implicitHeight: root.size

  Rectangle {
    anchors.fill: parent
    radius: width / 2
    color: root.tinted ? Util.alpha(root.tint, 0.22) : Util.alpha(Color.foreground, 0.1)

    Text {
      anchors.centerIn: parent
      text: root.text
      color: root.tinted ? root.tint : Color.foreground
      font.family: Theme.font.family
      font.pixelSize: Math.round(root.size * 0.4)
      font.weight: Font.DemiBold
    }
  }
}
