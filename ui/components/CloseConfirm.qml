import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// Asks what closing the window should do: keep OmaMessenger running in
// the background, so messages still notify, or quit until it is opened
// again. Tab moves between the buttons, Enter chooses, Escape cancels.
Item {
  id: root

  // open shows the question when true.
  property bool open: false

  // keep hides the window and keeps notifying.
  signal keep()
  // quit stops OmaMessenger until the window is opened again.
  signal quit()
  // cancelled leaves the window open.
  signal cancelled()

  // focusDefault puts keyboard focus on the safest choice.
  function focusDefault(): void {
    keepButton.forceActiveFocus();
  }

  objectName: "closeConfirm"
  anchors.fill: parent
  visible: root.open
  onOpenChanged: if (root.open) root.focusDefault()

  Rectangle {
    anchors.fill: parent
    color: Theme.menu.scrim

    MouseArea {
      anchors.fill: parent
      onClicked: root.cancelled()
    }
  }

  Ui.BorderSurface {
    id: card

    width: Math.min(parent.width - Theme.spacing.xxl * 2, Math.max(Style.space(420), buttons.implicitWidth + card.contentLeftInset + card.contentRightInset))
    height: content.implicitHeight + card.contentTopInset + card.contentBottomInset
    color: Theme.popups.background
    borderSpec: Border.flat(Theme.popups.border, Style.normalBorderWidth)
    radius: Style.cornerRadius
    padding: Theme.spacing.panelPadding
    anchors.centerIn: parent

    MouseArea {
      anchors.fill: parent
      onClicked: () => {}
    }

    ColumnLayout {
      id: content

      spacing: Theme.spacing.md
      anchors {
        fill: parent
        topMargin: card.contentTopInset
        rightMargin: card.contentRightInset
        bottomMargin: card.contentBottomInset
        leftMargin: card.contentLeftInset
      }

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
          objectName: "cancelButton"
          text: "Cancel"
          focusable: true
          onClicked: root.cancelled()
        }

        Ui.Button {
          objectName: "quitButton"
          text: "Quit"
          focusable: true
          onClicked: root.quit()
        }

        Ui.Button {
          id: keepButton

          objectName: "keepButton"
          text: "Keep in background"
          focusable: true
          selected: true
          onClicked: root.keep()
        }
      }
    }
  }
}
