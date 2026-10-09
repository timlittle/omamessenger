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
  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides); null uses Keymap's own defaults.
  property var bindings: Keymap.BINDINGS

  // hintsText joins each hinted binding's primary key and label.
  readonly property string hintsText: Keymap.bindingsFor(root.context, root.bindings)
    .map(b => `${Keymap.display(b.keys[0])} ${b.label}`)
    .join("   ")

  text: root.hintsText
  textFormat: Text.PlainText
  elide: Text.ElideRight
  color: Util.alpha(Color.foreground, 0.5)
  font.family: Theme.font.family
  font.pixelSize: Theme.font.caption
}
