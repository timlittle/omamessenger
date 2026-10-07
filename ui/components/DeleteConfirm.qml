import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// Asks what deleting the highlighted message should do: delete it for
// everyone (only offered for a message this account sent), remove it
// from this account's own view only, or leave it. h/l (or Left/Right)
// move a highlight across whichever choices apply, Enter chooses it, a
// mnemonic letter jumps straight to one, and Escape cancels. Cancel is
// always the default highlight, so a stray Enter never deletes
// anything. Every button stays clickable by mouse too.
Item {
  id: root

  // open shows the question when true.
  property bool open: false
  // targetIsOwn offers "Delete for everyone" only for a message this
  // account sent.
  property bool targetIsOwn: false
  // highlightIndex is the caller's highlighted choice, into whichever
  // choices targetIsOwn leaves on offer, left to right.
  property int highlightIndex: 0

  // routeKey is the panel's key router, so h/l, Enter and the mnemonic
  // letters work while this question holds no real keyboard focus of
  // its own control.
  property var routeKey: null

  // everyone deletes the message for everyone.
  signal everyone()
  // forMe removes the message from this account's own view only.
  signal forMe()
  // cancelled leaves the message as it is.
  signal cancelled()

  // _everyoneIndex and _forMeIndex are where those choices sit in
  // highlightIndex's order, matching DeleteController.choices: with
  // "everyone" on offer it comes first, otherwise "for me" does.
  readonly property int _everyoneIndex: 0
  readonly property int _forMeIndex: root.targetIsOwn ? 1 : 0
  readonly property int _cancelIndex: root.targetIsOwn ? 2 : 1

  objectName: "deleteConfirm"
  anchors.fill: parent
  visible: root.open
  focus: root.open

  Keys.onPressed: event => {
    if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
  }

  ModalCard {
    id: modal

    cardWidth: Math.max(Style.space(380), buttons.implicitWidth + modal.horizontalInsets)
    onOutsideClicked: root.cancelled()

    ColumnLayout {
      anchors.fill: parent
      spacing: Theme.spacing.md

      Text {
        text: "Delete this message?"
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        visible: root.targetIsOwn
        text: "You can delete it for everyone, or just remove it from your own view."
        wrapMode: Text.WordWrap
        color: Util.alpha(Color.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }

      RowLayout {
        id: buttons

        Layout.alignment: Qt.AlignRight
        spacing: Theme.spacing.controlGap

        Ui.Button {
          id: everyoneButton

          objectName: "everyoneButton"
          visible: root.targetIsOwn
          text: "e  Delete for everyone"
          selected: root.highlightIndex === root._everyoneIndex
          onClicked: root.everyone()
        }

        Ui.Button {
          id: forMeButton

          objectName: "forMeButton"
          text: "m  Delete for me"
          selected: root.highlightIndex === root._forMeIndex
          onClicked: root.forMe()
        }

        Ui.Button {
          id: cancelButton

          objectName: "cancelButton"
          text: "n  Cancel"
          selected: root.highlightIndex === root._cancelIndex
          onClicked: root.cancelled()
        }
      }
    }
  }
}
