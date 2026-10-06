import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// Message composer: a growing text input plus a Send button.
//
// This is a view only. It holds no helper or service references and makes
// no decision about *when* a message is sent beyond "the trimmed text is
// non-empty" — that call belongs to whoever wires it up.
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
  // routeKey intercepts a key press before the input handles it; see the
  // file comment above for why this is a function property, not a signal.
  property var routeKey: null

  // submitted reports the trimmed text a caller should send.
  signal submitted(string text)

  // submit sends the input's trimmed text, unless it is empty.
  function submit() {
    var trimmed = area.text.trim()
    if (trimmed.length === 0) return
    root.submitted(trimmed)
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

  RowLayout {
    id: layout
    anchors.fill: parent
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
