import QtQuick
import QtQuick.Controls
import QtQuick.Dialogs
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Format.js" as Format
import "../lib/Keymap.js" as Keymap
import "../lib/Mentions.js" as Mentions
import "../lib/SpellCheck.js" as SpellCheck

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
  // editTarget is the message text is non-null while the composer holds
  // an edit in progress rather than a new message: {id}, or null while
  // not editing. The caller has already loaded the message's own text
  // into text itself (see ComposerController.startEdit); this only
  // drives the banner and submit()'s send-or-save choice.
  property var editTarget: null
  // attachmentPath is the file to send with the next message, or "" for
  // none. The chip above the text field shows it.
  property string attachmentPath: ""
  // routeKey intercepts a key press before the input handles it; see the
  // file comment above for why this is a function property, not a signal.
  property var routeKey: null
  // members are the open group's members, for the @-mention picker; an
  // empty list for a direct chat, where "@" is just a character.
  property var members: []
  // misspellings are the composer's last spell.check result:
  // [{start, end, word}] in UTF-16 offsets, supplied by the caller
  // (Composer itself never calls the helper). Empty while spell check
  // is off or nothing is flagged.
  property var misspellings: []
  // suggestions are the last spell.suggest result the caller fetched
  // for whichever word the picker most recently asked about.
  property var suggestions: []

  // _mentionTokens are the members chosen through the picker so far, in
  // the order they were inserted, by id and name: enough for
  // Mentions.resolveMentions to find each one's final position in the
  // text once the message is submitted, however the text around it was
  // edited afterwards.
  property var _mentionTokens: []
  // _activeQuery is Mentions.activeQuery's result for the text and
  // caret right now, or null while the caret is not inside an "@query".
  property var _activeQuery: null
  // _mentionMatches are the members _activeQuery's query currently
  // matches, capped to a short list that fits on screen.
  property var _mentionMatches: []
  // _mentionIndex is which match the keyboard currently highlights.
  property int _mentionIndex: 0
  // pickerOpen is true while the @-mention picker should show.
  readonly property bool pickerOpen: root._activeQuery !== null && root._mentionMatches.length > 0
  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides), read for the mode hint below.
  property var bindings: Keymap.BINDINGS

  // _ignoredWords are words "Ignore" has dismissed for this session
  // only; never sent to the helper, and forgotten once the composer
  // closes (see docs/decisions.md).
  property var _ignoredWords: []
  // _effectiveMisspellings is misspellings with every ignored word
  // filtered out, the list every underline and picker decision reads.
  readonly property var _effectiveMisspellings: root.misspellings.filter(m => !root._ignoredWords.includes(m.word))
  // _spellActive is the misspelling range the spelling picker is open
  // for, or null while it is closed.
  property var _spellActive: null
  // _spellHighlightIndex is which picker row the keyboard currently
  // highlights: a suggestion index, or suggestions.length / +1 for
  // "Add to dictionary" / "Ignore".
  property int _spellHighlightIndex: 0
  // spellPickerOpen is true while the spelling picker should show.
  readonly property bool spellPickerOpen: root._spellActive !== null
  // _overlayRects are the underline rectangles to draw, in the text
  // input's own coordinates, recomputed by _recomputeOverlay.
  property var _overlayRects: []

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
  readonly property string modeHint: Keymap.composeHint(root.writing, root.bindings)

  // submitted reports the trimmed text a caller should send, alongside
  // whatever attachmentPath already holds, the id of the message it
  // answers (or "" when it answers nothing), and its resolved @-mentions
  // (or an empty list).
  signal submitted(string text, string replyToId, var mentions)
  // replyCanceled reports that the user dismissed the reply banner.
  signal replyCanceled()
  // editCanceled reports that the user dismissed the editing banner.
  signal editCanceled()
  // fileAttached reports a file the attach button's own picker chose.
  signal fileAttached(string path)
  // attachmentRemoveRequested asks the caller to clear attachmentPath,
  // from the chip's ✕ or Escape (handled by the caller's escape chain).
  signal attachmentRemoveRequested()
  // spellSuggestionsRequested asks the caller to fetch replacement
  // words for word, once the spelling picker opens over it.
  signal spellSuggestionsRequested(string word)
  // spellWordAdded asks the caller to add word to the personal
  // dictionary.
  signal spellWordAdded(string word)

  // submit sends the input's trimmed text and whatever is attached,
  // unless there is neither.
  function submit() {
    var trimmed = area.text.trim()
    if (trimmed.length === 0 && root.attachmentPath === "") return
    const mentions = Mentions.resolveMentions(trimmed, root._mentionTokens)
    root.submitted(trimmed, root.replyTo ? root.replyTo.id : "", mentions)
    area.text = ""
    root._mentionTokens = []
  }

  // focusInput moves keyboard focus into the text input.
  function focusInput() {
    area.forceActiveFocus()
  }

  // The caller's spell.check result can arrive after further typing
  // already moved the text layout, so it is recomputed here too, not
  // only from the text input's own onTextChanged.
  onMisspellingsChanged: root._recomputeOverlay()

  // _updateMentionQuery recomputes the picker's query and matches from
  // the input's current text and caret position; called whenever
  // either changes.
  function _updateMentionQuery() {
    const query = root.members.length > 0 ? Mentions.activeQuery(area.text, area.cursorPosition) : null
    root._activeQuery = query
    root._mentionMatches = query ? Mentions.filterMembers(root.members, query.query).slice(0, 6) : []
    root._mentionIndex = 0
  }

  // _runMentionAction performs one of the mentionPicker context's
  // actions (see Keymap.js); called only while root.pickerOpen.
  function _runMentionAction(action) {
    if (action === "mention.down") {
      root._mentionIndex = Math.min(root._mentionIndex + 1, root._mentionMatches.length - 1)
    } else if (action === "mention.up") {
      root._mentionIndex = Math.max(root._mentionIndex - 1, 0)
    } else if (action === "mention.accept") {
      root._acceptMention(root._mentionIndex)
    } else if (action === "mention.cancel") {
      root._activeQuery = null
      root._mentionMatches = []
    }
  }

  // _acceptMention inserts the matched member at index as a mention
  // token at the caret, and remembers it so submit() can resolve its
  // final position later.
  function _acceptMention(index) {
    const member = root._mentionMatches[index]
    if (!member) return

    const result = Mentions.insertMention(area.text, root._activeQuery.start, area.cursorPosition, member)
    root._mentionTokens = root._mentionTokens.concat([{ id: member.id, name: member.name }])
    area.text = result.text
    area.cursorPosition = result.cursor
  }

  // openFilePicker opens the attach button's file dialog, for the
  // command palette's "Attach a file" command.
  function openFilePicker() {
    fileDialog.open()
  }

  // _openSpellPickerAt opens the spelling picker for whichever
  // misspelling contains pos, a UTF-16 offset: the caret position for
  // the keyboard shortcut, or a clicked text position for a right-click.
  // Does nothing if pos is not inside a flagged word. Never opens while
  // IME composition is in progress, so a half-typed character is never
  // mistaken for a finished, misspelled word.
  function _openSpellPickerAt(pos) {
    if (area.inputMethodComposing) return

    const found = SpellCheck.misspellingAtPosition(root._effectiveMisspellings, pos) || SpellCheck.misspellingAtCaret(root._effectiveMisspellings, pos)
    if (!found) return

    root._spellActive = found
    root._spellHighlightIndex = 0
    root.spellSuggestionsRequested(found.word)
  }

  // _closeSpellPicker hides the spelling picker without changing
  // anything.
  function _closeSpellPicker() {
    root._spellActive = null
    root._spellHighlightIndex = 0
  }

  // _runSpellAction performs one of the spellPicker context's actions
  // (see Keymap.js); called only while root.spellPickerOpen.
  function _runSpellAction(action) {
    const rowCount = root.suggestions.length + 2

    if (action === "spell.down") {
      root._spellHighlightIndex = Math.min(root._spellHighlightIndex + 1, rowCount - 1)
    } else if (action === "spell.up") {
      root._spellHighlightIndex = Math.max(root._spellHighlightIndex - 1, 0)
    } else if (action === "spell.accept") {
      root._acceptSpellRow(root._spellHighlightIndex)
    } else if (action === "spell.addToDictionary") {
      root._addActiveWordToDictionary()
    } else if (action === "spell.ignore") {
      root._ignoreActiveWord()
    } else if (action === "spell.cancel") {
      root._closeSpellPicker()
    }
  }

  // _acceptSpellRow runs whichever row index names: a suggestion, "Add
  // to dictionary", or "Ignore", the same three choices a click on any
  // row reaches directly.
  function _acceptSpellRow(index) {
    if (index < root.suggestions.length) {
      root._applySpellSuggestion(index)
    } else if (index === root.suggestions.length) {
      root._addActiveWordToDictionary()
    } else {
      root._ignoreActiveWord()
    }
  }

  // _applySpellSuggestion replaces the active misspelling with
  // suggestions[index] and closes the picker.
  function _applySpellSuggestion(index) {
    const word = root.suggestions[index]
    if (!word || !root._spellActive) return

    const result = SpellCheck.applySuggestion(area.text, root._spellActive, word)
    area.text = result.text
    area.cursorPosition = result.cursor
    root._closeSpellPicker()
  }

  // _addActiveWordToDictionary asks the caller to add the active
  // word to the personal dictionary, and closes the picker; the
  // underline clears once the caller's recheck replies.
  function _addActiveWordToDictionary() {
    if (!root._spellActive) return

    root.spellWordAdded(root._spellActive.word)
    root._closeSpellPicker()
  }

  // _ignoreActiveWord dismisses the active word for this session only:
  // purely local, never sent to the helper. _overlayRects is rebuilt at
  // once, since _ignoredWords changing is not one of _recomputeOverlay's
  // other triggers (a text, caret or misspellings change).
  function _ignoreActiveWord() {
    if (!root._spellActive) return

    root._ignoredWords = root._ignoredWords.concat([root._spellActive.word])
    root._closeSpellPicker()
    root._recomputeOverlay()
  }

  // _recomputeOverlay rebuilds _overlayRects from _effectiveMisspellings,
  // deferred with Qt.callLater since the text layout is not settled in
  // the same tick a text or misspellings change arrives in.
  function _recomputeOverlay() {
    Qt.callLater(function() {
      let rects = []
      for (const m of root._effectiveMisspellings) rects = rects.concat(root._rectsForRange(m.start, m.end))
      root._overlayRects = rects
    })
  }

  // _rectsForRange returns the rectangle(s) to underline for the range
  // [start, end) in area's own coordinates: one rectangle, or two when
  // the range wraps across a line break.
  function _rectsForRange(start, end) {
    const from = area.positionToRectangle(start)
    const to = area.positionToRectangle(end)

    if (Math.abs(from.y - to.y) < 1) {
      return [{ x: from.x, y: from.y + from.height, width: Math.max(to.x - from.x, 1), height: 2 }]
    }

    return [
      { x: from.x, y: from.y + from.height, width: Math.max(area.width - area.rightPadding - from.x, 1), height: 2 },
      { x: area.leftPadding, y: to.y + to.height, width: Math.max(to.x - area.leftPadding, 1), height: 2 }
    ]
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
      textFormat: Text.PlainText
      color: root.writing ? Theme.accent : Util.alpha(Theme.foreground, 0.5)
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
        color: Theme.accent
      }

      Text {
        objectName: "replyBannerText"
        Layout.fillWidth: true
        elide: Text.ElideRight
        text: root.replyTo ? ("Replying to " + root.replyTo.senderName + ": " + root.replyTo.text) : ""
        // senderName and text both come from the other person, unescaped:
        // PlainText so a message that looks like HTML (an "<img src=...>",
        // say) is never auto-detected as rich text and never fetches
        // anything.
        textFormat: Text.PlainText
        color: Util.alpha(Theme.foreground, 0.7)
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
      id: editBanner
      objectName: "editBanner"
      Layout.fillWidth: true
      visible: root.editTarget !== null
      spacing: Theme.spacing.xs

      Rectangle {
        Layout.preferredWidth: Theme.spacing.xxs
        Layout.fillHeight: true
        radius: width / 2
        color: Theme.accent
      }

      Text {
        objectName: "editBannerText"
        Layout.fillWidth: true
        elide: Text.ElideRight
        text: "Editing message · Esc to cancel"
        textFormat: Text.PlainText
        color: Util.alpha(Theme.foreground, 0.7)
        font.family: Theme.font.family
        font.pixelSize: Theme.font.bodySmall
      }

      Ui.Button {
        objectName: "editCancel"
        text: "✕"
        tooltipText: "Cancel editing"
        focusable: true
        onClicked: root.editCanceled()
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
        color: Util.alpha(Theme.accent, 0.22)

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
          textFormat: Text.PlainText
          color: Theme.accent
          font { family: Theme.font.family; pixelSize: Theme.font.icon }
        }
      }

      Text {
        objectName: "attachmentName"
        Layout.fillWidth: true
        text: root.attachmentPath !== "" ? Format.baseName(root.attachmentPath) : ""
        textFormat: Text.PlainText
        elide: Text.ElideMiddle
        color: Theme.foreground
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

    MentionPicker {
      id: mentionPicker
      objectName: "mentionPicker"
      Layout.alignment: Qt.AlignLeft
      visible: root.pickerOpen
      members: root._mentionMatches
      highlightedIndex: root._mentionIndex
      onAccepted: index => root._acceptMention(index)
    }

    SpellPicker {
      id: spellPicker
      objectName: "spellPicker"
      Layout.alignment: Qt.AlignLeft
      visible: root.spellPickerOpen
      word: root._spellActive ? root._spellActive.word : ""
      suggestions: root.suggestions
      highlightedIndex: root._spellHighlightIndex
      onSuggestionChosen: index => root._applySpellSuggestion(index)
      onAddToDictionaryRequested: root._addActiveWordToDictionary()
      onIgnoreRequested: root._ignoreActiveWord()
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
          textFormat: TextEdit.PlainText
          selectByMouse: true
          placeholderText: root.title.length > 0 ? ("Message " + root.title) : "Message"

          onTextChanged: {
            root._updateMentionQuery()
            root._closeSpellPicker()
            root._recomputeOverlay()
          }
          onCursorPositionChanged: {
            root._updateMentionQuery()
            if (root.spellPickerOpen && (area.cursorPosition < root._spellActive.start || area.cursorPosition > root._spellActive.end)) {
              root._closeSpellPicker()
            }
          }

          Keys.priority: Keys.BeforeItem
          Keys.onPressed: event => {
            if (root.pickerOpen) {
              // Keymap.match also matches every global binding in this
              // context, not just the picker's own, so only a mention.*
              // action is handled here; anything else (Ctrl+N, Ctrl+/,
              // a rail switch, …) falls through to routeKey below
              // instead of being silently swallowed.
              const action = Keymap.match("mentionPicker", event.key, event.modifiers, event.text, root.bindings)
              if (action.startsWith("mention.")) {
                root._runMentionAction(action)
                event.accepted = true
                return
              }
            }
            if (root.spellPickerOpen) {
              const action = Keymap.match("spellPicker", event.key, event.modifiers, event.text, root.bindings)
              if (action.startsWith("spell.")) {
                root._runSpellAction(action)
                event.accepted = true
                return
              }
            }
            // Matched generically (not against a fixed "compose" context
            // pass-through) so every other compose binding - Enter,
            // Shift+Enter, Ctrl+V, … - keeps falling through to routeKey
            // exactly as before; only an exact match on this one action
            // is ever handled here.
            if (Keymap.match("compose", event.key, event.modifiers, event.text, root.bindings) === "compose.spellSuggestions") {
              root._openSpellPickerAt(area.cursorPosition)
              event.accepted = true
              return
            }
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
          }

          font.family: Theme.font.family
          font.pixelSize: Theme.font.body
          color: Theme.foreground
          placeholderTextColor: Util.alpha(Theme.foreground, 0.4)
          selectionColor: Style.selectionFill
          selectedTextColor: Theme.foreground

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

          // spellOverlay draws a wavy-in-spirit (a thin solid rule;
          // QML has no dashed/wavy line primitive) underline under each
          // misspelled range, recomputed by _recomputeOverlay whenever
          // the text, the caret or the misspellings themselves change,
          // so it tracks wrapped lines and edits.
          Item {
            id: spellOverlay
            objectName: "spellOverlay"
            anchors.fill: parent
            z: 1

            Repeater {
              model: root._overlayRects

              Rectangle {
                objectName: "spellUnderline-" + index
                x: modelData.x
                y: modelData.y
                width: modelData.width
                height: modelData.height
                color: Theme.urgent
              }
            }
          }

          // spellRightClick opens the spelling picker for the word
          // under a right-click, without otherwise changing how the
          // text input handles the mouse: a left-click, drag or
          // scroll passes straight through to it.
          MouseArea {
            id: spellRightClick
            objectName: "spellRightClickArea"
            anchors.fill: parent
            acceptedButtons: Qt.LeftButton | Qt.RightButton
            propagateComposedEvents: true
            onPressed: mouse => {
              if (mouse.button !== Qt.RightButton) {
                mouse.accepted = false
                return
              }

              root._openSpellPickerAt(area.positionAt(mouse.x, mouse.y))
              mouse.accepted = true
            }
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
