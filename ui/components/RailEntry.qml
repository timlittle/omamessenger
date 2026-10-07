import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Rail.js" as Rail

// One entry in the service rail: glyph, label, unread badge and a status
// dot, with a tooltip naming the entry and its status. Account entries are
// smaller and indented under their service.
Item {
  id: root

  // entry is one rail entry from Rail.items.
  property var entry: ({})
  // selected is true when the conversation list shows this entry.
  property bool selected: false
  // knownServices: the helper's own services, from hello, for the account
  // tooltip's service name; falls back to the built-in labels when empty.
  property var knownServices: []

  readonly property bool account: root.entry.kind === "account"
  readonly property string statusWord: Rail.statusLabel(root.entry.status)

  // clicked fires when the entry is clicked anywhere, glyph included.
  signal clicked()

  objectName: "entry-" + root.entry.key
  implicitHeight: layout.implicitHeight + Theme.spacing.sm * 2
  ToolTip.visible: hover.containsMouse
  ToolTip.text: (root.account ? Rail.accountLabel({ service: root.entry.service, name: root.entry.label }, root.knownServices) : root.entry.label)
    + (root.statusWord ? " · " + root.statusWord : "")
  ToolTip.delay: 500

  Rectangle {
    anchors.fill: parent
    color: root.selected ? Util.alpha(Color.accent, Style.selectedFillAlpha)
      : hover.containsMouse ? Style.hoverFill : "transparent"
  }

  // The selection bar matches the conversation list's.
  Rectangle {
    visible: root.selected
    width: Style.space(2)
    color: Color.accent
    anchors { left: parent.left; top: parent.top; bottom: parent.bottom }
  }

  ColumnLayout {
    id: layout

    spacing: Theme.spacing.xxs
    anchors { centerIn: parent; horizontalCenterOffset: root.account ? Theme.spacing.xs : 0 }

    // The icon cell: the glyph, its status dot, and the unread badge. The
    // badge sits outside the glyph's own box, to its top right, so a busy
    // count never squashes into the icon it is counting for. Only its left
    // edge is anchored to the glyph, which alone keeps the two apart: a
    // badge can sit anywhere above that edge without ever reaching back
    // over the glyph.
    Item {
      Layout.alignment: Qt.AlignHCenter
      implicitWidth: glyph.implicitWidth + (badge.visible ? Theme.spacing.xxs + badge.implicitWidth : 0)
      implicitHeight: Math.max(glyph.implicitHeight, badge.implicitHeight)

      ServiceGlyph {
        id: glyph

        objectName: "glyph"
        anchors { left: parent.left; verticalCenter: parent.verticalCenter }
        service: root.entry.service ?? ""
      }

      // A hollow ring while connecting, a solid urgent dot on error.
      Rectangle {
        visible: root.entry.status === "connecting" || root.entry.status === "error" || root.entry.status === "needs-auth"
        width: Style.space(8)
        height: Style.space(8)
        radius: width / 2
        color: root.entry.status === "error" || root.entry.status === "needs-auth" ? Color.urgent : "transparent"
        border { width: root.entry.status === "connecting" ? Theme.spacing.hairline : 0; color: Color.foreground }
        anchors { right: glyph.right; bottom: glyph.bottom }
      }

      UnreadBadge {
        id: badge

        objectName: "badge"
        count: root.entry.unread ?? 0
        anchors { left: glyph.right; top: parent.top; leftMargin: Theme.spacing.xxs }
      }
    }

    Text {
      objectName: "label"
      Layout.alignment: Qt.AlignHCenter
      Layout.maximumWidth: root.width - Theme.spacing.xs * 2
      text: root.entry.label ?? ""
      color: Color.foreground
      elide: Text.ElideRight
      font { family: Theme.font.family; pixelSize: root.account ? Theme.font.caption : Theme.font.bodySmall }
    }
  }

  // Above the glyph and label, so a click anywhere selects the entry and
  // there is one tooltip rather than the glyph's own as well.
  MouseArea {
    id: hover

    anchors.fill: parent
    hoverEnabled: true
    onClicked: root.clicked()
  }
}
