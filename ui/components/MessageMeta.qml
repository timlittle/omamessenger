pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// The bubble's time/status row: the sent time, an "edited" label if the
// message was edited, and the delivery glyph for an outgoing message
// that has not failed. Split out of MessageDelegate to keep that file
// within the size guideline.
Row {
  id: root

  // message carries created, edited, status and outgoing, as
  // MessageDelegate reads them. It can be reset straight to a QML null
  // while the delegate it belongs to is destroyed, so every binding
  // below guards it with "&&" rather than trusting required to mean
  // non-null for the binding's whole lifetime.
  required property var message
  // nowMs is the current time, read for the sent-time label below.
  // Defaults to the shared clock; a test overrides it to pick an
  // arbitrary time.
  property real nowMs: Clock.nowMs
  // showStatus is true when the delivery glyph should be drawn.
  required property bool showStatus

  spacing: Theme.spacing.xs
  layoutDirection: root.message && root.message.outgoing ? Qt.RightToLeft : Qt.LeftToRight

  Text {
    text: root.message ? Format.timeLabel(root.message.created, root.nowMs) : ""
    color: Util.alpha(Color.foreground, 0.5)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  Text {
    visible: root.message && root.message.edited === true
    text: "edited"
    color: Util.alpha(Color.foreground, 0.5)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  Text {
    visible: root.showStatus
    text: root.message ? Format.statusGlyph(root.message.status) : ""
    color: root.message && root.message.status === "read" ? Color.accent : Util.alpha(Color.foreground, 0.5)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }
}
