import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"

// A compact quote above a reply's own text: the quoted message's sender
// and one line of its text, with an accent bar like a link preview.
// Clicking it asks to scroll to the quoted message, if it is loaded.
Item {
  id: root

  // reply is the message's replyTo: remoteId, senderName and text (an
  // excerpt), as the helper sends it.
  required property var reply

  // opened reports the quoted message's remote id, to scroll to it.
  signal opened(string remoteId)

  implicitWidth: Style.space(280)
  implicitHeight: column.implicitHeight + Theme.spacing.xs * 2

  // A bar in the accent color marks the quote as belonging to the reply,
  // the same way a link preview is marked.
  Rectangle {
    width: Theme.spacing.xxs
    height: parent.height
    radius: width / 2
    color: Color.accent
  }

  ColumnLayout {
    id: column

    spacing: Theme.spacing.xxs
    anchors {
      left: parent.left
      right: parent.right
      verticalCenter: parent.verticalCenter
      leftMargin: Theme.spacing.sm
    }

    Text {
      Layout.fillWidth: true
      visible: text !== ""
      text: root.reply.senderName || ""
      // The quoted message's sender name and excerpt are both
      // sender-controlled, unescaped: PlainText.
      textFormat: Text.PlainText
      elide: Text.ElideRight
      color: Color.accent
      font { family: Theme.font.family; pixelSize: Theme.font.caption; weight: Font.DemiBold }
    }

    Text {
      Layout.fillWidth: true
      visible: text !== ""
      text: root.reply.text || ""
      textFormat: Text.PlainText
      elide: Text.ElideRight
      maximumLineCount: 1
      color: Util.alpha(Color.foreground, 0.7)
      font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
    }
  }

  MouseArea {
    anchors.fill: parent
    cursorShape: Qt.PointingHandCursor
    onClicked: root.opened(root.reply.remoteId)
  }
}
