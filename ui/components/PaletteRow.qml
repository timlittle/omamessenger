import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"

// One row of the command palette: what it does, an optional detail, and
// its shortcut on the right.
Item {
  id: root

  // item is {label, detail, keys}.
  property var item: ({})
  // current highlights the row the keyboard is on.
  property bool current: false

  // chosen fires when the row is clicked.
  signal chosen()

  implicitHeight: row.implicitHeight + Theme.spacing.sm * 2

  Rectangle {
    anchors.fill: parent
    radius: Style.cornerRadius
    color: root.current ? Util.alpha(Color.accent, Style.selectedFillAlpha)
      : hover.containsMouse ? Style.hoverFill : "transparent"
  }

  RowLayout {
    id: row

    spacing: Theme.spacing.md
    anchors { fill: parent; leftMargin: Theme.spacing.sm; rightMargin: Theme.spacing.sm }

    Text {
      Layout.maximumWidth: row.width * 0.7
      text: root.item.label ?? ""
      color: Color.foreground
      elide: Text.ElideRight
      font { family: Theme.font.family; pixelSize: Theme.font.body }
    }

    Text {
      Layout.fillWidth: true
      text: root.item.detail ?? ""
      color: Util.alpha(Color.foreground, 0.5)
      elide: Text.ElideRight
      font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
    }

    // A conversation row's own unread count, the same pill the list shows.
    UnreadBadge {
      objectName: "unreadBadge"
      count: root.item.unread ?? 0
    }

    Text {
      visible: text !== ""
      text: root.item.keys ?? ""
      color: Util.alpha(Color.foreground, 0.6)
      font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
    }
  }

  MouseArea {
    id: hover

    anchors.fill: parent
    hoverEnabled: true
    onClicked: root.chosen()
  }
}
