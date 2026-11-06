import QtQuick
import qs.Ui as Ui
import "../theme"

// A service's Nerd Font glyph, always paired with a tooltip naming it.
Item {
  id: root

  property string service: ""
  // U+F0FCC (md-message_lock) is outside the BMP, so it is written as
  // a surrogate pair rather than a literal private-use character.
  readonly property string text: root.service === "whatsapp" ? ""
    : root.service === "telegram" ? ""
    : root.service === "signal" ? "\uDB83\uDFCC" : ""
  readonly property string label: root.service === "whatsapp" ? "WhatsApp"
    : root.service === "telegram" ? "Telegram"
    : root.service === "signal" ? "Signal" : "Messages"

  implicitWidth: glyph.implicitWidth
  implicitHeight: glyph.implicitHeight

  Text {
    id: glyph
    text: root.text
    textFormat: Text.PlainText
    color: Theme.foreground
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
