import QtQuick
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// A contact's initials in a quiet circle.
Item {
  id: root

  property string name: ""
  property int size: Style.space(32)
  readonly property string text: Format.initials(root.name)

  implicitWidth: root.size
  implicitHeight: root.size

  Rectangle {
    anchors.fill: parent
    radius: width / 2
    color: Util.alpha(Color.foreground, 0.1)

    Text {
      anchors.centerIn: parent
      text: root.text
      color: Color.foreground
      font.family: Theme.font.family
      font.pixelSize: Math.round(root.size * 0.4)
      font.weight: Font.DemiBold
    }
  }
}
