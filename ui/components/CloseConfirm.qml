import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// Asks what closing the window should do: keep OmaMessenger running in
// the background, so messages still notify, or quit until it is opened
// again. h/l (or Left/Right) move a highlight across Cancel, Quit and
// Keep, Enter chooses it, a mnemonic letter jumps straight to one, and
// Escape cancels. Every button stays clickable by mouse too.
Item {
  id: root

  // open shows the question when true.
  property bool open: false
  // highlightIndex is the caller's highlighted choice: 0 Cancel, 1 Quit,
  // 2 Keep in background, left to right.
  property int highlightIndex: 2
  // routeKey is the panel's key router, so h/l, Enter and the mnemonic
  // letters work while this question holds no real keyboard focus of
  // its own control.
  property var routeKey: null

  // keep hides the window and keeps notifying.
  signal keep()
  // quit stops OmaMessenger until the window is opened again.
  signal quit()
  // cancelled leaves the window open.
  signal cancelled()

  objectName: "closeConfirm"
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
      id: content

      anchors.fill: parent
      spacing: Theme.spacing.md

      Text {
        text: "Close OmaMessenger?"
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        text: "Keep it running to get notifications for new messages, or quit until you open it again."
        wrapMode: Text.WordWrap
        color: Util.alpha(Color.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }

      RowLayout {
        id: buttons

        Layout.alignment: Qt.AlignRight
        spacing: Theme.spacing.controlGap

        Ui.Button {
          id: cancelButton

          objectName: "cancelButton"
          text: "c  Cancel"
          selected: root.highlightIndex === 0
          onClicked: root.cancelled()
        }

        Ui.Button {
          id: quitButton

          objectName: "quitButton"
          text: "q  Quit"
          selected: root.highlightIndex === 1
          onClicked: root.quit()
        }

        Ui.Button {
          id: keepButton

          objectName: "keepButton"
          text: "k  Keep in background"
          selected: root.highlightIndex === 2
          onClicked: root.keep()
        }
      }
    }
  }
}
