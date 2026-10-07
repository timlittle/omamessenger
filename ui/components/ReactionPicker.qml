pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"

// A short picker of common emoji, for adding a reaction without typing
// one: arrow keys move the highlight, Enter picks it, Escape cancels. The
// same picker opens from a message's "+" chip or the "react to the
// newest message" command, so it carries no message state of its own.
Item {
  id: root

  // open shows the picker when true.
  property bool open: false
  // emojis are the choices shown, in order.
  property var emojis: []
  // currentIndex is the highlighted emoji.
  property int currentIndex: 0
  // routeKey is the panel's key router, so arrow keys and Enter work
  // while this picker holds no real keyboard focus of its own.
  property var routeKey: null

  // picked reports the emoji at index was chosen.
  signal picked(int index)
  // cancelled asks the caller to close the picker without reacting.
  signal cancelled()

  objectName: "reactionPicker"
  anchors.fill: parent
  visible: root.open
  focus: root.open

  Keys.onPressed: event => {
    if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true;
  }

  ModalCard {
    id: modal
    objectName: "reactionPickerModal"
    cardName: "reactionPickerCard"
    cardWidth: row.implicitWidth + Theme.spacing.panelPadding * 2
    cardHeight: row.implicitHeight + Theme.spacing.panelPadding * 2
    onOutsideClicked: root.cancelled()

    RowLayout {
      id: row
      anchors.centerIn: parent
      spacing: Theme.spacing.sm

      Repeater {
        model: root.emojis

        Rectangle {
          id: slot
          required property string modelData
          required property int index

          readonly property bool current: slot.index === root.currentIndex

          objectName: "slot-" + slot.index
          Layout.preferredWidth: Theme.spacing.controlHeight
          Layout.preferredHeight: Theme.spacing.controlHeight
          radius: Style.cornerRadius
          color: slot.current ? Util.alpha(Color.accent, Style.selectedFillAlpha)
            : hover.containsMouse ? Style.hoverFill : "transparent"
          border.width: slot.current ? Theme.spacing.hairline : 0
          border.color: Color.accent

          Text {
            anchors.centerIn: parent
            text: slot.modelData
            font.pixelSize: Theme.font.title
          }

          MouseArea {
            id: hover
            objectName: "slotArea-" + slot.index
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: Qt.PointingHandCursor
            onClicked: root.picked(slot.index)
          }
        }
      }
    }
  }
}
