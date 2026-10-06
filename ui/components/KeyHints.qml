import QtQuick
import qs.Commons
import "../theme"
import "../lib/Keymap.js" as Keymap

// One-line footer of key hints for the active context, elided when the
// window is too narrow to show them all.
Text {
  id: root

  // context is the active key context, e.g. "list" or "conversation".
  property string context: ""

  // hintsText joins each hinted binding's primary key and label.
  readonly property string hintsText: Keymap.bindingsFor(root.context)
    .map(b => `${b.keys[0]} ${b.label}`)
    .join("   ")

  text: root.hintsText
  elide: Text.ElideRight
  color: Util.alpha(Color.foreground, 0.5)
  font.family: Theme.font.family
  font.pixelSize: Theme.font.caption
}
