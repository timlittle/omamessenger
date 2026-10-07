import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// Message composer: an optional "replying to" banner, a growing text
// input and a Send button.
//
// This is a view only. It holds no helper or service references and makes
// no decision about *when* a message is sent beyond "the trimmed text is
// non-empty" — that call belongs to whoever wires it up. replyTo is data
// the caller hands in and gets back through submitted(); the composer
// shows it and lets the user cancel it, nothing more.
//
// Enter / Shift+Enter are deliberately NOT handled here. routeKey, when
// set, is called with a key press's (key, modifiers, text) before the
// input does anything with it; returning true marks the key handled, so
// Enter can call submit() instead of inserting a newline. It takes the
// event's raw fields rather than the KeyEvent itself: a KeyEvent copies
// when it crosses a signal, and mutating a copy's `accepted` would not
// stop the real one, so the input sets `accepted` itself from the
// boolean this returns. The `input` alias lets the same caller watch
// activeFocus and focus the field itself.
Item {
  id: root

  // ------------------------------------------------------------- API
  property string title: ""
  property alias text: area.text
  property alias input: area
  // replyTo is the message being answered: {id, senderName, text}, or
  // null when the user is not replying to anything.
  property var replyTo: null
  // routeKey intercepts a key press before the input handles it; see the
  // file comment above for why this is a function property, not a signal.
  property var routeKey: null

  // submitted reports the trimmed text a caller should send, and the id
  // of the message it answers, or "" when it answers nothing.
  signal submitted(string text, string replyToId)
  // replyCanceled reports that the user dismissed the reply banner.
  signal replyCanceled()

  // submit sends the input's trimmed text, unless it is empty.
  function submit() {
    var trimmed = area.text.trim()
    if (trimmed.length === 0) return
    root.submitted(trimmed, root.replyTo ? root.replyTo.id : "")
    area.text = ""
  }

  // focusInput moves keyboard focus into the text input.
  function focusInput() {
    area.forceActiveFocus()
  }

  // ------------------------------------------------------------- sizing
  readonly property int maxVisibleLines: 6
  readonly property real _lineHeight: fontMetrics.height
  readonly property real _maxInputHeight: _lineHeight * maxVisibleLines + area.topPadding + area.bottomPadding
  readonly property bool _hasText: area.text.trim().length > 0

  implicitWidth: Style.space(280)
  implicitHeight: layout.implicitHeight

  opacity: root.enabled ? 1.0 : 0.5
  Behavior on opacity { NumberAnimation { duration: 120 } }

  FontMetrics {
    id: fontMetrics
    font.family: Theme.font.family
    font.pixelSize: Theme.font.body
  }

  ColumnLayout {
    id: layout
    anchors.fill: parent
    spacing: Theme.spacing.xs

    RowLayout {
      id: replyBanner
      objectName: "replyBanner"
      Layout.fillWidth: true
      visible: root.replyTo !== null
      spacing: Theme.spacing.xs

      Rectangle {
        Layout.preferredWidth: Theme.spacing.xxs
        Layout.fillHeight: true
        radius: width / 2
        color: Color.accent
      }

      Text {
        objectName: "replyBannerText"
        Layout.fillWidth: true
        elide: Text.ElideRight
        text: root.replyTo ? ("Replying to " + root.replyTo.senderName + ": " + root.replyTo.text) : ""
        color: Util.alpha(Color.foreground, 0.7)
        font.family: Theme.font.family
        font.pixelSize: Theme.font.bodySmall
      }

      Text {
        objectName: "replyCancel"
        text: "✕"
        color: Util.alpha(Color.foreground, 0.6)
        font.family: Theme.font.family
        font.pixelSize: Theme.font.bodySmall

        HoverHandler { id: cancelHover }
        ToolTip.visible: cancelHover.hovered
        ToolTip.text: "Cancel reply"
        ToolTip.delay: 500

        MouseArea {
          anchors.fill: parent
          cursorShape: Qt.PointingHandCursor
          onClicked: root.replyCanceled()
        }
      }
    }

    RowLayout {
      Layout.fillWidth: true
      spacing: Theme.spacing.controlGap

      ScrollView {
        id: inputScroll
        Layout.fillWidth: true
        Layout.preferredHeight: Math.min(area.implicitHeight, root._maxInputHeight)
        clip: true
        ScrollBar.vertical.policy: area.implicitHeight > root._maxInputHeight ? ScrollBar.AsNeeded : ScrollBar.AlwaysOff

        TextArea {
          id: area
          objectName: "composerInput"
          enabled: root.enabled
          wrapMode: TextEdit.WrapAtWordBoundaryOrAnywhere
          selectByMouse: true
          placeholderText: root.title.length > 0 ? ("Message " + root.title) : "Message"

          Keys.priority: Keys.BeforeItem
          Keys.onPressed: event => {
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
          }

          font.family: Theme.font.family
          font.pixelSize: Theme.font.body
          color: Color.foreground
          placeholderTextColor: Util.alpha(Color.foreground, 0.4)
          selectionColor: Style.selectionFill
          selectedTextColor: Color.foreground

          topPadding: Theme.spacing.inputPaddingY
          bottomPadding: Theme.spacing.inputPaddingY
          leftPadding: Theme.spacing.controlPaddingX
          rightPadding: Theme.spacing.controlPaddingX

          background: Rectangle {
            radius: Style.cornerRadius
            // Omarchy's control-state tokens, so the input matches its own fields.
            color: area.activeFocus ? Style.focusFillColor : Style.normalFill
            border.width: area.activeFocus ? Style.focusBorderWidth : 0
            border.color: area.activeFocus ? Style.focusBorderColor : "transparent"

            Behavior on color { ColorAnimation { duration: 120 } }
          }
        }
      }

      Ui.Button {
        id: sendButton
        Layout.alignment: Qt.AlignBottom
        text: "Send"
        focusable: true
        enabled: root.enabled && root._hasText
        onClicked: root.submit()
      }
    }
  }
}
