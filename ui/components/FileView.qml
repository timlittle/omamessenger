import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// A file in a message: a badge with its extension, its name, and its size.
// A click downloads it if need be and opens it in the user's application
// for that kind of file.
Item {
  id: root

  // file is the message's file media: fileName, size and duration.
  required property var file

  // opened asks for the file to be opened, downloading it first if need be.
  signal opened()

  // _extension is the file's extension in capitals, or a general label.
  readonly property string _extension: {
    const name = root.file.fileName || "";
    const dot = name.lastIndexOf(".");
    return dot > 0 && name.length - dot <= 5 ? name.slice(dot + 1).toUpperCase() : "FILE";
  }

  implicitWidth: Style.space(280)
  implicitHeight: row.implicitHeight

  RowLayout {
    id: row

    anchors { left: parent.left; right: parent.right }
    spacing: Theme.spacing.sm

    Rectangle {
      Layout.preferredWidth: Style.space(40)
      Layout.preferredHeight: Style.space(40)
      radius: Style.cornerRadius
      color: Util.alpha(Color.accent, 0.22)

      Text {
        anchors.centerIn: parent
        text: root._extension
        color: Color.accent
        font { family: Theme.font.family; pixelSize: Theme.font.caption; weight: Font.DemiBold }
      }
    }

    ColumnLayout {
      Layout.fillWidth: true
      spacing: 0

      Text {
        Layout.fillWidth: true
        text: root.file.fileName || "File"
        elide: Text.ElideMiddle
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        visible: text !== ""
        text: [Format.fileSize(root.file.size), Format.duration(root.file.duration)].filter((part) => part !== "").join(" · ")
        color: Util.alpha(Color.foreground, 0.6)
        font { family: Theme.font.family; pixelSize: Theme.font.caption }
      }
    }
  }

  MouseArea {
    anchors.fill: parent
    cursorShape: Qt.PointingHandCursor
    onClicked: root.opened()
  }
}
