import QtQuick
import qs.Commons
import qs.Ui as Ui
import "../theme"

// A service's Nerd Font glyph, always paired with a tooltip naming it.
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
    textFormat: Text.PlainText
    color: Color.foreground
    font.family: Theme.font.family
    font.pixelSize: Theme.font.icon
  }

  MouseArea {
    id: hover
    anchors.fill: parent
    hoverEnabled: true
  }

  Ui.PanelToolTip {
    visible: hover.containsMouse
    text: root.label
  }
}
