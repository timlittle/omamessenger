import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"

// One contact row in the new-chat dialog: avatar and name, highlighted
// when it is the keyboard-selected row or under the pointer. A helper
// split out of NewChatDialog to keep that file within the component size
// guideline.
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
  color: fill.color

  HighlightFill {
    id: fill
    selected: root.current
    hovered: hover.containsMouse
  }

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
      // A contact's name is whatever they or the service set it to:
      // PlainText.
      textFormat: Text.PlainText
      elide: Text.ElideRight
      color: Color.foreground
      font.family: Theme.font.family
      font.pixelSize: Theme.font.body
    }
  }

  MouseArea {
    id: hover

    anchors.fill: parent
    hoverEnabled: true
    cursorShape: Qt.PointingHandCursor
    onClicked: root.chosen()
  }
}
