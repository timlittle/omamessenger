import QtQuick
import QtQuick.Controls
import QtQuick.Dialogs
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Format.js" as Format
import "../lib/Keymap.js" as Keymap

// Message composer: an optional "replying to" banner, an attachment
// chip, a growing text input and a Send button.
//
// This is a view only. It holds no helper or service references and makes
// no decision about *when* a message is sent beyond "there is text or an
// attachment" — that call belongs to whoever wires it up. attachmentPath
// and replyTo are supplied by the caller, the same way title and draft
// text are: the attach button's own file picker and a pasted image both
// need no protocol call, but a pasted image does (media.paste), and a
// reply is chosen from the conversation above, so the caller decides
// what each turned out to be and hands it back down, exactly like draft
// text already flows out through submitted() and back in through text.
//
// Enter, Shift+Enter and Ctrl+J are deliberately NOT handled here.
// routeKey, when set, is called with a key press's (key, modifiers, text)
// before the input does anything with it; returning true marks the key
// handled, so Enter can call submit() instead of inserting a newline,
// Shift+Enter and Ctrl+J can insert a newline explicitly (Ctrl+J would
// otherwise mean "next unread conversation", see Keymap.js's compose
// context), and Ctrl+V can ask the caller to check the clipboard for an
// image instead of pasting text. It takes the event's raw fields rather
// than the KeyEvent itself:
// a KeyEvent copies when it crosses a signal, and mutating a copy's
// `accepted` would not stop the real one, so the input sets `accepted`
// itself from the boolean this returns. The `input` alias lets the same
// caller watch activeFocus and focus the field itself.
Item {
  id: root
  objectName: "composer"

  // ------------------------------------------------------------- API
  property string title: ""
  property alias text: area.text
  property alias input: area
  // replyTo is the message being answered: {id, senderName, text}, or
  // null when the user is not replying to anything.
  property var replyTo: null
  // attachmentPath is the file to send with the next message, or "" for
  // none. The chip above the text field shows it.
  property string attachmentPath: ""
  // routeKey intercepts a key press before the input handles it; see the
  // file comment above for why this is a function property, not a signal.
  property var routeKey: null

  // writing is true while the text input holds keyboard focus: typing
  // and scrolling the conversation look identical otherwise, bar the
  // blinking text cursor, so the mode hint, the input and Send all read
  // this to show which one is active.
  readonly property bool writing: area.activeFocus
  // dimmedOpacity is how faded the input and Send look while scrolling,
  // not writing. It is visual only: both stay enabled and clickable, so
  // clicking the input starts writing and Send still works by mouse.
  readonly property real dimmedOpacity: 0.5
  // modeHint names the current mode in words, so it is not shown by
  // colour alone: Keymap.composeHint derives it from the same bindings
  // the key router already matches, rather than naming a key twice.
  readonly property string modeHint: Keymap.composeHint(root.writing)

  // submitted reports the trimmed text a caller should send, alongside
  // whatever attachmentPath already holds, and the id of the message it
  // answers, or "" when it answers nothing.
  signal submitted(string text, string replyToId)
  // replyCanceled reports that the user dismissed the reply banner.
  signal replyCanceled()
  // fileAttached reports a file the attach button's own picker chose.
  signal fileAttached(string path)
  // attachmentRemoveRequested asks the caller to clear attachmentPath,
  // from the chip's ✕ or Escape (handled by the caller's escape chain).
  signal attachmentRemoveRequested()

  // submit sends the input's trimmed text and whatever is attached,
  // unless there is neither.
  function submit() {
    var trimmed = area.text.trim()
    if (trimmed.length === 0 && root.attachmentPath === "") return
    root.submitted(trimmed, root.replyTo ? root.replyTo.id : "")
    area.text = ""
  }

  // focusInput moves keyboard focus into the text input.
  function focusInput() {
    area.forceActiveFocus()
  }

  // openFilePicker opens the attach button's file dialog, for the
  // command palette's "Attach a file" command.
  function openFilePicker() {
    fileDialog.open()
  }

  // ------------------------------------------------------------- sizing
  readonly property int maxVisibleLines: 6
  readonly property real _lineHeight: fontMetrics.height
  readonly property real _maxInputHeight: _lineHeight * maxVisibleLines + area.topPadding + area.bottomPadding
  readonly property bool _canSend: area.text.trim().length > 0 || root.attachmentPath !== ""
  // _attachmentKind guesses the chip's thumbnail-or-name choice from the
  // file name alone, before the helper ever sniffs its real content.
  readonly property string _attachmentKind: root.attachmentPath !== "" ? Format.guessMediaKind(root.attachmentPath) : ""

  implicitWidth: Style.space(280)
  implicitHeight: layout.implicitHeight + 2 * Theme.spacing.xs

  opacity: root.enabled ? 1.0 : 0.5
  Behavior on opacity { NumberAnimation { duration: 120 } }

  FontMetrics {
    id: fontMetrics
    font.family: Theme.font.family
    font.pixelSize: Theme.font.body
  }

  FileDialog {
    id: fileDialog
    title: "Attach a file"
    onAccepted: root.fileAttached(String(fileDialog.selectedFile).replace(/^file:\/\//, ""))
  }

  ColumnLayout {
    id: layout
    anchors.fill: parent
    anchors.margins: Theme.spacing.xs
    spacing: Theme.spacing.xs

    Text {
      id: modeHintText
      objectName: "composerModeHint"
      Layout.fillWidth: true
      horizontalAlignment: Text.AlignRight
      elide: Text.ElideRight
      text: root.modeHint
      color: root.writing ? Color.accent : Util.alpha(Color.foreground, 0.5)
      font.family: Theme.font.family
      font.pixelSize: Theme.font.caption
    }

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

      Ui.Button {
        objectName: "replyCancel"
        text: "✕"
        tooltipText: "Cancel reply"
        focusable: true
        onClicked: root.replyCanceled()
      }
    }

    RowLayout {
      id: chipRow
      objectName: "attachmentChip"
      Layout.fillWidth: true
      visible: root.attachmentPath !== ""
      spacing: Theme.spacing.sm

      Rectangle {
        Layout.preferredWidth: Style.space(32)
        Layout.preferredHeight: Style.space(32)
        radius: Style.cornerRadius
        clip: true
        color: Util.alpha(Color.accent, 0.22)

        Image {
          anchors.fill: parent
          visible: root._attachmentKind === "photo"
          source: root._attachmentKind === "photo" ? ("file://" + root.attachmentPath) : ""
          fillMode: Image.PreserveAspectCrop
          asynchronous: true
        }

        Text {
          anchors.centerIn: parent
          visible: root._attachmentKind !== "photo"
          text: root._attachmentKind === "video" ? "▶" : "📄"
          color: Color.accent
          font { family: Theme.font.family; pixelSize: Theme.font.icon }
        }
      }

      Text {
        objectName: "attachmentName"
        Layout.fillWidth: true
        text: root.attachmentPath !== "" ? Format.baseName(root.attachmentPath) : ""
        elide: Text.ElideMiddle
        color: Color.foreground
        font.family: Theme.font.family
        font.pixelSize: Theme.font.bodySmall
      }

      Ui.Button {
        objectName: "removeAttachmentButton"
        text: "✕"
        tooltipText: "Remove the attachment"
        focusable: true
        onClicked: root.attachmentRemoveRequested()
      }
    }

    RowLayout {
      Layout.fillWidth: true
      spacing: Theme.spacing.controlGap

      Ui.Button {
        id: attachButton
        objectName: "attachButton"
        Layout.alignment: Qt.AlignBottom
        text: "📎"
        tooltipText: "Attach a file"
        focusable: true
        enabled: root.enabled
        onClicked: root.openFilePicker()
      }

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
          // Dimmed while scrolling rather than writing, so the mode is
          // visible at a glance without relying on the hint text alone.
          // The input stays enabled: clicking it while dimmed still
          // starts writing.
          opacity: root.writing ? 1.0 : root.dimmedOpacity
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
        objectName: "sendButton"
        Layout.alignment: Qt.AlignBottom
        text: "Send"
        focusable: true
        enabled: root.enabled && root._canSend
        // Dimmed while scrolling rather than writing; stays clickable,
        // so Send still works by mouse even before the input has focus.
        opacity: root.writing ? 1.0 : root.dimmedOpacity
        onClicked: root.submit()
      }
    }
  }
}
