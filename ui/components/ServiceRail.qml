pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
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
  // demo: true while the helper is seeded with demo data.
  property bool demo: false

  // selected fires when an entry is clicked.
  signal selected(string key)
  // newChat fires when the "+" button is clicked.
  signal newChat()
  // help fires when the "?" button is clicked.
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

    // DEMO chip: shown only while the helper is seeded, never implying a
    // real account is connected.
    Rectangle {
      id: demoChip
      objectName: "demoChip"
      visible: root.demo
      Layout.alignment: Qt.AlignHCenter
      Layout.bottomMargin: Theme.spacing.xs
      implicitWidth: demoLabel.implicitWidth + Theme.spacing.sm * 2
      implicitHeight: demoLabel.implicitHeight + Theme.spacing.xxs * 2
      radius: implicitHeight / 2
      color: Util.alpha(Color.urgent, 0.25)

      Text {
        id: demoLabel
        anchors.centerIn: parent
        text: "DEMO"
        color: Color.foreground
        font.family: Theme.font.family
        font.pixelSize: Theme.font.caption
        font.weight: Font.Bold
      }

      MouseArea {
        id: demoHover
        anchors.fill: parent
        hoverEnabled: true
      }
      ToolTip.visible: demoHover.containsMouse
      ToolTip.text: "Seeded demo data. WhatsApp and Telegram are not connected."
      ToolTip.delay: 500
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
      tooltipText: "Shortcuts · F1"
      onClicked: root.help()
    }
  }
}
