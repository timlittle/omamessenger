import QtQuick
import qs.Commons
import "../lib/Format.js" as Format

// Avatar circle with contact initials.
//
// Displays a circular avatar containing the initials of a contact name.
// The size property controls both width and height. Uses neutral background
// and foreground colors from the theme.
Item {
  id: root

  // ------------------------------------------------------------- API
  property string name: ""
  property int size: 32
  readonly property string text: Format.initials(root.name)

  // ------------------------------------------------------------- impl
  implicitWidth: root.size
  implicitHeight: root.size

  Rectangle {
    anchors.fill: parent
    radius: root.size / 2
    color: Util.alpha(Color.foreground, 0.1)

    Text {
      anchors.centerIn: parent
      text: root.text
      color: Color.foreground
      font.pixelSize: Math.round(root.size / 2.4)
      font.weight: Font.DemiBold
      horizontalAlignment: Text.AlignHCenter
      verticalAlignment: Text.AlignVCenter
    }
  }
}
