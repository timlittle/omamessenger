import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"

// A quiet footer reminder that incognito read receipts are on: this
// account never tells its service a chat here was read, so the phone
// and any other device signed into the same account keep showing it
// unread even once it has been read here. Pairs a glyph with a label
// rather than colour alone, matching how the list's own mute glyph and
// tooltip work.
RowLayout {
  id: root

  // active shows the indicator; false while read receipts are on.
  property bool active: false

  spacing: Theme.spacing.xs
  visible: root.active

  Text {
    objectName: "readReceiptsGlyph"
    text: "🙈"
    textFormat: Text.PlainText
    font.pixelSize: Theme.font.caption
  }

  Text {
    objectName: "readReceiptsLabel"
    text: "Read receipts off"
    textFormat: Text.PlainText
    color: Util.alpha(Color.foreground, 0.6)
    font.family: Theme.font.family
    font.pixelSize: Theme.font.caption
  }
}
