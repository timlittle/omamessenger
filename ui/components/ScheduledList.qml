pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Format.js" as Format

// The open conversation's own queued "send later" messages, shown as a
// quiet strip between the message list and the composer: a clock glyph,
// when each is due, and its text, with buttons to send it now, edit it
// or cancel it. Hidden outright while there is nothing queued for this
// conversation.
Column {
  id: root

  // messages are this conversation's own scheduled messages, soonest
  // due first: [{id, text, sendAt}].
  property var messages: []

  // nowMs is the current time, read for each row's "Sends at" label.
  // Defaults to the shared clock; a test overrides it to pick an
  // arbitrary time (see MessageMeta.qml's own property of the same name).
  property real nowMs: Clock.nowMs

  // cancelRequested asks the caller to cancel a queued message.
  signal cancelRequested(string id)
  // sendNowRequested asks the caller to send a queued message right away.
  signal sendNowRequested(string id)
  // editRequested asks the caller to load a queued message back into
  // the composer for editing.
  signal editRequested(string id, string text)

  visible: root.messages.length > 0
  width: parent ? parent.width : 0
  spacing: Theme.spacing.xxs

  Repeater {
    model: root.messages

    Ui.BorderSurface {
      id: row
      required property var modelData

      width: root.width
      implicitHeight: content.implicitHeight + Theme.spacing.xs * 2
      color: Theme.popups.background
      borderSpec: Border.flat(Theme.popups.border, Style.normalBorderWidth)
      radius: Style.cornerRadius

      RowLayout {
        id: content
        anchors {
          left: parent.left
          right: parent.right
          verticalCenter: parent.verticalCenter
          leftMargin: Theme.spacing.sm
          rightMargin: Theme.spacing.xs
        }
        spacing: Theme.spacing.sm

        Text {
          text: "🕐"
          textFormat: Text.PlainText
          font.pixelSize: Theme.font.body
        }

        ColumnLayout {
          Layout.fillWidth: true
          spacing: Theme.spacing.xxs

          Text {
            Layout.fillWidth: true
            text: Format.sendAtLabel(row.modelData.sendAt, root.nowMs)
            textFormat: Text.PlainText
            color: Theme.accent
            font { family: Theme.font.family; pixelSize: Theme.font.caption; weight: Font.DemiBold }
          }

          Text {
            Layout.fillWidth: true
            text: row.modelData.text
            textFormat: Text.PlainText
            elide: Text.ElideRight
            maximumLineCount: 1
            color: Util.alpha(Theme.foreground, 0.8)
            font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
          }
        }

        Ui.PanelActionButton {
          objectName: "scheduledSendNowButton"
          iconText: "➤"
          tooltipText: "Send now"
          size: Theme.spacing.lg
          onClicked: root.sendNowRequested(row.modelData.id)
        }

        Ui.PanelActionButton {
          objectName: "scheduledEditButton"
          iconText: "✎"
          tooltipText: "Edit"
          size: Theme.spacing.lg
          onClicked: root.editRequested(row.modelData.id, row.modelData.text)
        }

        Ui.PanelActionButton {
          objectName: "scheduledCancelButton"
          iconText: "✕"
          tooltipText: "Cancel"
          size: Theme.spacing.lg
          onClicked: root.cancelRequested(row.modelData.id)
        }
      }
    }
  }
}
