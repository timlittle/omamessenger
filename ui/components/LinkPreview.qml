import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"

// A link's preview under a message: the site, the page's title and the
// start of its description, with the small picture Telegram sent. All of
// it comes with the message; nothing is fetched from the site. A click
// opens the link.
Item {
  id: root

  // preview is the message's link media: url, siteName, title,
  // description and thumb, a base64 JPEG.
  required property var preview

  // opened reports the link to open.
  signal opened(string url)

  implicitWidth: Style.space(360)
  implicitHeight: row.implicitHeight + Theme.spacing.xs * 2

  // A bar in the accent color marks the card as belonging to the link.
  Rectangle {
    width: Theme.spacing.xxs
    height: parent.height
    radius: width / 2
    color: Color.accent
  }

  RowLayout {
    id: row

    spacing: Theme.spacing.sm
    anchors {
      left: parent.left
      right: parent.right
      verticalCenter: parent.verticalCenter
      leftMargin: Theme.spacing.sm
    }

    ColumnLayout {
      Layout.fillWidth: true
      spacing: Theme.spacing.xxs

      Text {
        Layout.fillWidth: true
        visible: text !== ""
        text: root.preview.siteName || ""
        // The whole preview comes from the message's sender (or
        // whatever the link itself fed back to their client); none of
        // it is ever escaped, so every label here stays PlainText.
        textFormat: Text.PlainText
        elide: Text.ElideRight
        color: Color.accent
        font { family: Theme.font.family; pixelSize: Theme.font.caption; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        visible: text !== ""
        text: root.preview.title || ""
        textFormat: Text.PlainText
        wrapMode: Text.Wrap
        maximumLineCount: 2
        elide: Text.ElideRight
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        visible: text !== ""
        text: root.preview.description || ""
        textFormat: Text.PlainText
        wrapMode: Text.Wrap
        maximumLineCount: 3
        elide: Text.ElideRight
        color: Util.alpha(Color.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
      }
    }

    Image {
      Layout.preferredWidth: Style.space(56)
      Layout.preferredHeight: Style.space(56)
      Layout.alignment: Qt.AlignTop
      visible: !!root.preview.thumb
      // thumb is a base64 JPEG the connector already downloaded and
      // embedded in the message; this never fetches a remote URL.
      source: root.preview.thumb ? "data:image/jpeg;base64," + root.preview.thumb : ""
      fillMode: Image.PreserveAspectCrop
    }
  }

  MouseArea {
    anchors.fill: parent
    cursorShape: Qt.PointingHandCursor
    onClicked: root.opened(root.preview.url)
  }
}
