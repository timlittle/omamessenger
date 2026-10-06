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
// Enter / Shift+Enter are deliberately NOT handled here. The Panel forwards
// key events to a router, which calls submit() or focusInput() on this
// component as needed. The `input` alias lets that router (or the
// Panel) attach its own Keys handlers directly to the inner TextArea.
Item {
  id: root

  // ------------------------------------------------------------- API
  property string title: ""
  property alias text: area.text
  property alias input: area

  signal submitted(string text)

  function submit() {
    var trimmed = area.text.trim()
    if (trimmed.length === 0) return
    root.submitted(trimmed)
    area.text = ""
  }

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
        enabled: root.enabled
        wrapMode: TextEdit.WrapAtWordBoundaryOrAnywhere
        selectByMouse: true
        placeholderText: root.title.length > 0 ? ("Message " + root.title) : "Message"

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
