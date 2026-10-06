import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui

// Message composer: a growing text input plus a Send button.
//
// This is a view only. It holds no RPC/service references (C10) and makes
// no decision about *when* a message is sent beyond "the trimmed text is
// non-empty" — that call belongs to whoever wires it up.
//
// Enter / Shift+Enter are deliberately NOT handled here. The Panel forwards
// key events to a router, which calls submit() or focusInput() on this
// component as needed (C6). The `input` alias lets that router (or the
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
    font.family: Style.font.family
    font.pixelSize: Style.font.body
  }

  RowLayout {
    id: layout
    anchors.fill: parent
    spacing: Style.spacing.controlGap

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

        font.family: Style.font.family
        font.pixelSize: Style.font.body
        color: Color.foreground
        placeholderTextColor: Util.alpha(Color.foreground, 0.4)
        selectionColor: Util.alpha(Color.accent, 0.35)
        selectedTextColor: Color.foreground

        topPadding: Style.spacing.inputPaddingY
        bottomPadding: Style.spacing.inputPaddingY
        leftPadding: Style.spacing.controlPaddingX
        rightPadding: Style.spacing.controlPaddingX

        background: Rectangle {
          radius: Style.cornerRadius
          color: area.activeFocus ? Util.alpha(Color.foreground, 0.08) : Util.alpha(Color.foreground, 0.04)
          border.width: area.activeFocus ? Style.space(1) : 0
          border.color: area.activeFocus ? Util.alpha(Color.accent, 0.6) : "transparent"

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
