import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"

// One contact row in the new-chat dialog: avatar and name, highlighted
// when it is the keyboard-selected row. A helper split out of
// NewChatDialog to keep that file within the component size guideline.
Rectangle {
  id: root

  // contact is one Contact: accountId, remoteId, name.
  required property var contact
  // current is true when this row is the keyboard-selected one.
  property bool current: false

  // chosen reports that this row was clicked to open a conversation.
  signal chosen()

  height: Theme.spacing.popupRowHeight * 1.5
  radius: Style.cornerRadius
  color: root.current ? Util.alpha(Color.accent, Style.selectedFillAlpha) : "transparent"

  RowLayout {
    anchors.fill: parent
    anchors.margins: Theme.spacing.sm
    spacing: Theme.spacing.controlGap

    Avatar {
      name: root.contact.name
      size: Style.space(24)
    }

    Text {
      Layout.fillWidth: true
      text: root.contact.name
      elide: Text.ElideRight
      color: Color.foreground
      font.family: Theme.font.family
      font.pixelSize: Theme.font.body
    }
  }

  MouseArea {
    anchors.fill: parent
    cursorShape: Qt.PointingHandCursor
    onClicked: root.chosen()
  }
}
