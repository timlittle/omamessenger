pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import qs.Commons
import "../theme"

// The reaction chips under a message bubble: one per emoji with its
// count. The user's own reaction gets the accent colour, a border and a
// bolder weight, so it still reads without relying on colour alone, plus
// a tooltip saying so. A quiet "+" chip appears while hovering is true,
// to open the emoji picker.
Item {
  id: root

  // reactions are the message's chips: {emoji, count, mine}. The caller
  // may hand this null or undefined: a message with none, or a delegate
  // whose message is being torn down between recycles, so it is never
  // read directly below.
  required property var reactions
  // hovering shows the "+" chip even when there are no reactions yet.
  property bool hovering: false
  // alignRight puts the chips under an outgoing message's right-aligned
  // bubble.
  property bool alignRight: false

  // toggled asks the caller to add or remove emoji as the user's reaction.
  signal toggled(string emoji)
  // addRequested asks the caller to open the emoji picker for this message.
  signal addRequested()

  // visible guards reactions with "&&" rather than a separate normalizing
  // property: a message's reactions can be reset straight to a QML null
  // while its delegate is destroyed, bypassing whatever an intermediate
  // property's own binding would have computed, so the null must be
  // caught in the same expression that reads .length.
  visible: (root.reactions && root.reactions.length > 0) || root.hovering
  implicitWidth: row.implicitWidth
  implicitHeight: row.implicitHeight

  Row {
    id: row
    anchors.right: root.alignRight ? parent.right : undefined
    anchors.left: root.alignRight ? undefined : parent.left
    spacing: Theme.spacing.xxs

    Repeater {
      model: root.reactions ? root.reactions : []

      Rectangle {
        id: chip
        required property var modelData

        objectName: "chip-" + chip.modelData.emoji
        radius: Style.cornerRadius
        color: chip.modelData.mine ? Util.alpha(Color.accent, Style.selectedFillAlpha)
          : chipHover.containsMouse ? Style.hoverFill : Util.alpha(Color.foreground, 0.06)
        border.width: chip.modelData.mine ? Theme.spacing.hairline : 0
        border.color: Color.accent
        width: chipText.implicitWidth + Theme.spacing.sm * 2
        height: chipText.implicitHeight + Theme.spacing.xxs * 2
        ToolTip.visible: chip.modelData.mine && chipHover.containsMouse
        ToolTip.text: "You reacted"
        ToolTip.delay: 500

        Text {
          id: chipText
          anchors.centerIn: parent
          text: chip.modelData.emoji + " " + chip.modelData.count
          color: Color.foreground
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption
          font.weight: chip.modelData.mine ? Font.DemiBold : Font.Normal
        }

        MouseArea {
          id: chipHover
          objectName: "chipArea-" + chip.modelData.emoji
          anchors.fill: parent
          hoverEnabled: true
          cursorShape: Qt.PointingHandCursor
          onClicked: root.toggled(chip.modelData.emoji)
        }
      }
    }

    Rectangle {
      id: addChip
      objectName: "addChip"
      visible: root.hovering
      radius: Style.cornerRadius
      color: addHover.containsMouse ? Style.hoverFill : "transparent"
      width: addText.implicitWidth + Theme.spacing.sm * 2
      height: addText.implicitHeight + Theme.spacing.xxs * 2

      Text {
        id: addText
        anchors.centerIn: parent
        text: "+"
        color: Util.alpha(Color.foreground, 0.7)
        font.family: Theme.font.family
        font.pixelSize: Theme.font.caption
      }

      MouseArea {
        id: addHover
        objectName: "addChipArea"
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: root.addRequested()
      }
    }
  }
}
