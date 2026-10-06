import QtQuick
import QtQuick.Controls
import qs.Commons

// Service provider icon with tooltip.
//
// Displays a Nerd Font icon representing a messaging service (WhatsApp,
// Telegram, or a generic fallback). Includes a tooltip naming the service.
Item {
  id: root

  // ------------------------------------------------------------- API
  property string service: ""
  readonly property string text: {
    switch (root.service) {
      case "whatsapp":
        return "";
      case "telegram":
        return "";
      default:
        return "";
    }
  }

  // ------------------------------------------------------------- impl
  implicitWidth: glyph.implicitWidth
  implicitHeight: glyph.implicitHeight

  Text {
    id: glyph
    text: root.text
    color: Color.foreground
    font.pixelSize: 16
    font.family: "Noto Sans Mono"
    horizontalAlignment: Text.AlignHCenter
    verticalAlignment: Text.AlignVCenter
  }

  ToolTip {
    visible: mouseArea.containsMouse
    text: {
      switch (root.service) {
        case "whatsapp":
          return "WhatsApp";
        case "telegram":
          return "Telegram";
        default:
          return root.service.charAt(0).toUpperCase() + root.service.slice(1);
      }
    }
    delay: 500
    timeout: 5000
  }

  MouseArea {
    id: mouseArea
    anchors.fill: parent
    hoverEnabled: true
  }
}
