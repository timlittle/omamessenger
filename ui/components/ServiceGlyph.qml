import QtQuick
import QtQuick.Controls
import qs.Commons
import "../theme"

// A service's Nerd Font glyph (C8), always paired with a tooltip naming it.
Item {
  id: root

  property string service: ""
  readonly property string text: root.service === "whatsapp" ? ""
    : root.service === "telegram" ? "" : ""
  readonly property string label: root.service === "whatsapp" ? "WhatsApp"
    : root.service === "telegram" ? "Telegram" : "Messages"

  implicitWidth: glyph.implicitWidth
  implicitHeight: glyph.implicitHeight

  Text {
    id: glyph
    text: root.text
    color: Color.foreground
    font.family: Theme.font.family
    font.pixelSize: Theme.font.icon
  }

  MouseArea {
    id: hover
    anchors.fill: parent
    hoverEnabled: true
  }

  ToolTip.visible: hover.containsMouse
  ToolTip.text: root.label
  ToolTip.delay: 500
}
