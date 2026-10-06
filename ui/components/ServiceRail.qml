pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// The service rail: an "All" entry, each service with at least one
// account, and that service's accounts when it has more than one. Shows
// unread totals, connection status and shortcuts to start a chat or open
// help. This is a view only: it reports intent through signals and never
// calls the helper.
Item {
  id: root

  // items: rail entries from Rail.items(accounts, conversations).
  property var items: []
  // selectedKey: the entry currently shown in the conversation list.
  property string selectedKey: ""

  // selected fires when an entry is clicked.
  signal selected(string key)
  // newChat fires when the "+" button is clicked.
  signal newChat()
  // help fires when the "?" button is clicked; it opens the command palette.
  signal help()

  implicitWidth: Style.space(64)
  implicitHeight: column.implicitHeight

  ColumnLayout {
    id: column
    anchors.fill: parent
    spacing: Theme.spacing.xs

    ListView {
      id: list
      Layout.fillWidth: true
      Layout.fillHeight: true
      clip: true
      model: root.items
      spacing: Theme.spacing.xxs
      delegate: RailEntry {
        required property var modelData

        width: list.width
        entry: modelData
        selected: modelData.key === root.selectedKey
        onClicked: root.selected(modelData.key)
      }
    }

    Ui.Button {
      Layout.alignment: Qt.AlignHCenter
      objectName: "newChatButton"
      text: "+"
      focusable: true
      tooltipText: "New chat · Ctrl+N"
      onClicked: root.newChat()
    }

    Ui.Button {
      Layout.alignment: Qt.AlignHCenter
      objectName: "helpButton"
      text: "?"
      focusable: true
      tooltipText: "Commands · Ctrl+/"
      onClicked: root.help()
    }
  }
}
