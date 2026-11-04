pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Format.js" as Format
import "../lib/Keymap.js" as Keymap
import "../lib/ScheduledAction.js" as ScheduledAction

// The open conversation's queued "send later" messages, shown as a
// strip between the message list and composer, with send-now/edit/
// cancel buttons. Hidden when nothing is queued.
Column {
  id: root

  // messages are this conversation's own scheduled messages, soonest
  // due first: [{id, text, sendAt}].
  property var messages: []

  // nowMs is the current time, used for each row's "Sends at" label.
  property real nowMs: Clock.nowMs

  // highlightedId is the queued message j/k currently point at, or ""
  // while none of these rows is highlighted.
  property string highlightedId: ""

  // bindings are the effective key bindings, for the highlighted row's hints.
  property var bindings: Keymap.BINDINGS

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

      // highlighted is true when the keyboard points at this row.
      readonly property bool highlighted: row.modelData.id === root.highlightedId

      objectName: "scheduledRow-" + row.modelData.id
      width: root.width
      implicitHeight: content.implicitHeight + Theme.spacing.xs * 2
      color: row.highlighted ? Util.alpha(Theme.accent, Style.selectedFillAlpha) : Theme.popups.background
      borderSpec: row.highlighted
        ? Border.flat(Theme.accent, Style.focusBorderWidth + Style.normalBorderWidth)
        : Border.flat(Theme.popups.border, Style.normalBorderWidth)
      radius: Style.cornerRadius

      ColumnLayout {
        id: content
        anchors {
          left: parent.left
          right: parent.right
          verticalCenter: parent.verticalCenter
          leftMargin: Theme.spacing.sm
          rightMargin: Theme.spacing.xs
        }
        spacing: Theme.spacing.xxs

        RowLayout {
          Layout.fillWidth: true
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

        // Always takes its row's space so toggling opacity never
        // shifts other rows.
        Text {
          objectName: "scheduledHighlightHints"
          Layout.fillWidth: true
          opacity: row.highlighted ? 1 : 0
          text: ScheduledAction.hints(root.bindings)
          textFormat: Text.PlainText
          color: Util.alpha(Theme.foreground, 0.5)
          font.family: Theme.font.family
          font.pixelSize: Theme.font.bodySmall
        }
      }
    }
  }
}
