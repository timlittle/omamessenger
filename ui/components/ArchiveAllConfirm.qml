import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// Asks whether to archive every read, unpinned conversation in the
// current list, since it is a bulk action: Enter or y confirms, n or
// Esc cancels, the same y/n idiom RemoveAccount's own question uses.
Item {
  id: root

  // open shows the question when true.
  property bool open: false
  // count is how many conversations this would archive, named in the
  // question so the user knows the scope before confirming.
  property int count: 0
  // routeKey is the panel's key router, so Enter/y and n work while this
  // question holds no real keyboard focus of its own control.
  property var routeKey: null

  // confirmed archives the conversations the question named.
  signal confirmed()
  // cancelled leaves every conversation as it is.
  signal cancelled()

  objectName: "archiveAllConfirm"
  anchors.fill: parent
  visible: root.open
  focus: root.open

  Keys.onPressed: event => {
    if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
  }

  ModalCard {
    id: modal

    cardWidth: Math.max(Style.space(420), buttons.implicitWidth + modal.horizontalInsets)
    onOutsideClicked: root.cancelled()

    ColumnLayout {
      anchors.fill: parent
      spacing: Theme.spacing.md

      Text {
        objectName: "archiveAllQuestion"
        text: "Archive " + root.count + " read conversation" + (root.count === 1 ? "" : "s") + "?"
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        text: "Pinned and unread conversations are left as they are."
        wrapMode: Text.WordWrap
        color: Util.alpha(Color.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }

      RowLayout {
        id: buttons

        Layout.alignment: Qt.AlignRight
        spacing: Theme.spacing.controlGap

        Ui.Button {
          objectName: "cancelButton"
          text: "n  Cancel"
          onClicked: root.cancelled()
        }

        Ui.Button {
          objectName: "archiveButton"
          text: "y  Archive"
          selected: true
          onClicked: root.confirmed()
        }
      }
    }
  }
}
