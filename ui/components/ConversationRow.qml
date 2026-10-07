import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// One row in the conversation list: avatar, title, time, a preview line or
// a highlighted search match, the owning account when more than one
// account shares a service, a pin mark, and a mute or unread indicator. A
// pinned chat is marked with a glyph and tooltip; an archived chat's
// unread badge is dimmed like a muted one's, since the chat is already
// filed away, though its count still adds to the rail's unread total. A
// row shown only because show-all is on (older, hidden or archived) is
// drawn with reduced opacity and a small "Hidden" or "Archived" label, so
// the chats someone usually looks at still stand out without relying on
// opacity alone.
// This is a view only: it reports intent through a signal and never
// calls the helper.
Item {
  id: root

  // ------------------------------------------------------------- API
  // conversation: the helper's Conversation fields for this row.
  property var conversation: null
  // selected: true when this row is the open conversation.
  property bool selected: false
  // query: the active search text, used to show a highlighted match.
  property string query: ""
  // showAccount: true when the owning service has more than one account.
  property bool showAccount: false
  // accountName: the owning account's name, shown when showAccount is true.
  property string accountName: ""
  // nowMs: the current time, for the relative time label.
  property real nowMs: Date.now()
  // dimmed: true when this row is shown only because show-all is on (it
  // would not appear in the standard list): drawn with reduced opacity.
  property bool dimmed: false
  // dimLabel: why a dimmed row would not normally show, "Hidden" or
  // "Archived"; "" for a row dimmed only for being older, or one that is
  // not dimmed at all.
  property string dimLabel: ""

  // clicked fires when the row is clicked.
  signal clicked()

  readonly property var _conv: root.conversation ?? {}
  readonly property bool _unread: (root._conv.unread ?? 0) > 0
  readonly property bool _muted: root._conv.muted ?? false
  readonly property bool _pinned: root._conv.pinned ?? false
  readonly property bool _archived: root._conv.archived ?? false
  readonly property string _previewHtml: root.query.length > 0 && root._conv.match
    ? Format.highlight(Format.escapeHtml(root._conv.match), root.query)
    : Format.escapeHtml(Format.previewLine(root._conv))

  objectName: "row-" + (root._conv.id ?? "")
  implicitWidth: Style.space(260)
  implicitHeight: Style.space(60)
  opacity: root.dimmed ? 0.6 : 1.0
  Behavior on opacity { NumberAnimation { duration: 120 } }

  Rectangle {
    anchors.fill: parent
    color: root.selected ? Util.alpha(Color.accent, Style.selectedFillAlpha)
      : hover.containsMouse ? Style.hoverFill : "transparent"
    Behavior on color { ColorAnimation { duration: 120 } }
  }

  // Selection bar: a 2px accent stripe on the left.
  Rectangle {
    visible: root.selected
    anchors.left: parent.left
    anchors.top: parent.top
    anchors.bottom: parent.bottom
    width: Style.space(2)
    color: Color.accent
  }

  MouseArea {
    id: hover
    anchors.fill: parent
    hoverEnabled: true
    onClicked: root.clicked()
  }

  RowLayout {
    anchors.fill: parent
    anchors.leftMargin: Theme.spacing.md
    anchors.rightMargin: Theme.spacing.md
    spacing: Theme.spacing.sm

    Avatar {
      name: root._conv.title ?? ""
      size: Style.space(40)
    }

    ColumnLayout {
      Layout.fillWidth: true
      spacing: Theme.spacing.xxs

      RowLayout {
        Layout.fillWidth: true
        spacing: Theme.spacing.xs

        Text {
          objectName: "titleText"
          Layout.fillWidth: true
          text: root._conv.title ?? ""
          color: Color.foreground
          font.family: Theme.font.family
          font.pixelSize: Theme.font.body
          font.bold: root._unread
          elide: Text.ElideRight
        }

        Text {
          objectName: "pinIcon"
          visible: root._pinned
          text: ""
          color: Util.alpha(Color.foreground, 0.5)
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption

          // A passive hover handler, like the mute glyph below: it must
          // not swallow the row's own click.
          HoverHandler {
            id: pinHover
          }
          ToolTip.visible: pinHover.hovered
          ToolTip.text: "Pinned"
          ToolTip.delay: 500
        }

        Text {
          objectName: "dimLabel"
          visible: root.dimLabel.length > 0
          text: root.dimLabel
          color: Util.alpha(Color.foreground, 0.5)
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption
        }

        Text {
          text: Format.timeLabel(root._conv.lastActivity ?? 0, root.nowMs)
          color: Util.alpha(Color.foreground, 0.6)
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption
        }
      }

      RowLayout {
        Layout.fillWidth: true
        spacing: Theme.spacing.xs

        ServiceGlyph {
          visible: root.showAccount
          service: root._conv.service ?? ""
        }

        Text {
          visible: root.showAccount
          text: root.accountName
          color: Util.alpha(Color.foreground, 0.6)
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption
        }

        Text {
          objectName: "previewText"
          Layout.fillWidth: true
          textFormat: Text.StyledText
          text: root._previewHtml
          color: Util.alpha(Color.foreground, 0.7)
          font.family: Theme.font.family
          font.pixelSize: Theme.font.bodySmall
          elide: Text.ElideRight
        }

        Text {
          objectName: "muteIcon"
          visible: root._muted
          text: ""
          color: Util.alpha(Color.foreground, 0.5)
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption

          // A passive hover handler, not a MouseArea: it must not swallow
          // the row's own click when the mute glyph sits under the
          // pointer.
          HoverHandler {
            id: muteHover
          }
          ToolTip.visible: muteHover.hovered
          ToolTip.text: "Muted"
          ToolTip.delay: 500
        }

        UnreadBadge {
          objectName: "unreadBadge"
          count: root._conv.unread ?? 0
          muted: root._muted || root._archived
        }
      }
    }
  }
}
