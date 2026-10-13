import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Format.js" as Format
import "../lib/Rpc.js" as Rpc
import "../lib/SpellCheck.js" as SpellCheck

// Owns the composer's state for the open conversation: its draft text,
// the message it is replying to, a pasted or attached file waiting to be
// sent, whether it holds keyboard focus, and the spell check round trip
// (Composer.qml draws the underlines and the picker from what this
// controller reports; it never calls the helper itself, see qml.md).
//
// Item rather than QtObject: spellCheckTimer below needs a child object.
Item {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // conversation is the ConversationController this composer belongs to.
  property var conversation: null

  // draft is the open conversation's unsent composer text.
  property string draft: ""

  // replyTarget is the message the composer is about to answer: {id,
  // senderName, text}, or null when the user is not replying to anything.
  property var replyTarget: null

  // replying is true while replyTarget names a message, for the Escape
  // chain: Escape cancels the reply before it leaves the composer.
  readonly property bool replying: root.replyTarget !== null

  // attachmentPath is the file to send with the next message, or "" for
  // none: set by attachFile (the composer's own file picker) or
  // pasteImage (a clipboard image, through the helper), and read by
  // ConversationController.send(). See Composer.qml for why the caller,
  // not the composer, owns it.
  property string attachmentPath: ""

  // composeFocused mirrors whether the composer holds keyboard focus; the
  // caller sets it from the real text field so the Escape chain can read it.
  property bool composeFocused: false

  // composeFocusRequested asks the caller to focus the composer.
  signal composeFocusRequested()

  // attachFileRequested asks the caller to open the composer's file
  // picker, for the command palette's "Attach a file" command.
  signal attachFileRequested()

  // submitRequested asks the caller to submit whatever the composer holds.
  signal submitRequested()

  // newlineRequested asks the caller to insert a line break at the
  // composer's cursor, for Shift+Enter and Ctrl+J: both mean "new line"
  // while composing, even though Ctrl+J means "next unread conversation"
  // everywhere else (see Keymap.js's compose context).
  signal newlineRequested()

  // pasteFallbackRequested asks the caller to paste the clipboard's text
  // into the composer, because pasteImage found no image there and
  // Ctrl+V must still work as a plain text paste.
  signal pasteFallbackRequested()

  // leaveComposeRequested asks the caller to move focus out of the composer.
  signal leaveComposeRequested()

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "composer";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "compose.focus": () => root.composeFocusRequested(),
      "message.reply": () => root.startReply(root.conversation.highlightedId),
      "message.send": () => root.submitRequested(),
      "compose.newline": () => root.newlineRequested(),
      "compose.attach": () => root.pasteImage(),
      "compose.attachFile": () => root.attachFileRequested()
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // setDraft stores the composer's text for the open conversation, so it
  // survives the panel being recreated.
  function setDraft(text: string): void {
    root.draft = text;
    if (!root.conversation.activeId) return;

    root.conversation.saveUiState({ drafts: Object.assign({}, root.service.uiState.drafts, { [root.conversation.activeId]: text }) });
  }

  // startReply makes id, a loaded message, the one the composer answers
  // next. Nothing changes if it is not loaded.
  function startReply(id: string): void {
    const m = root.conversation.timeline.messageById(id);
    if (!m) return;

    root.replyTarget = { id: m.id, senderName: m.outgoing ? "You" : m.senderName, text: Format.singleLine(m.text) };
  }

  // cancelReply clears the composer's reply target without sending.
  function cancelReply(): void {
    root.replyTarget = null;
  }

  // attachFile records a file the composer's own file picker chose, to
  // send with the next message.
  function attachFile(path: string): void {
    root.attachmentPath = path;
  }

  // removeAttachment clears whatever attachFile or pasteImage set, from
  // the composer's chip or the Escape chain.
  function removeAttachment(): void {
    root.attachmentPath = "";
  }

  // pasteImage asks the helper whether the clipboard holds an image; if
  // it does, it is attached to the next message, and otherwise Ctrl+V
  // still pastes text, same as it always did. media.paste answers
  // asynchronously, so the open conversation is captured when the paste
  // starts; if it has since changed by the time the reply lands, the
  // reply is dropped outright rather than attaching the pasted image, or
  // falling back to a plain paste, to whichever conversation now happens
  // to be open (see docs/decisions.md).
  function pasteImage(): void {
    if (!root.service) return;

    const conversationId = root.conversation ? root.conversation.activeId : "";
    const isCurrent = Rpc.guard(() => root.conversation ? root.conversation.activeId : null, conversationId);
    root.service.request("media.paste", {}, function(error, result) {
      if (!isCurrent()) return;
      if (error) { root.pasteFallbackRequested(); return; }
      root.attachmentPath = result.path;
    });
  }

  // reset clears the reply and any attachment, without touching the
  // draft: ConversationController calls this when the open conversation
  // closes or switches to another one.
  function reset(): void {
    root.replyTarget = null;
    root.attachmentPath = "";
    root.spellMisspellings = [];
    root.spellSuggestions = [];
  }

  // restore loads id's saved draft and clears any reply or attachment
  // left over from whatever conversation was open before.
  function restore(id: string): void {
    root.reset();
    root.draft = root.draftFor(id);
  }

  // draftFor reads a conversation's saved draft out of uiState.
  function draftFor(id: string): string {
    const drafts = root.service.uiState.drafts || {};
    return drafts[id] || "";
  }

  // spellMisspellings is the last spell.check result for the composer's
  // text: [{start, end, word}], in UTF-16 offsets. Composer.qml draws
  // the underlines from this.
  property var spellMisspellings: []

  // spellSuggestions is the last spell.suggest result, for whichever
  // word the picker most recently asked about.
  property var spellSuggestions: []

  // _lastSpellText is the composer text spellCheckTimer's debounce is
  // about to check, or just checked; addSpellingWord re-checks it
  // directly once a word is added, since adding one does not itself
  // change the text and so would otherwise never trigger a recheck.
  property string _lastSpellText: ""

  // requestSpellCheck asks the helper to re-check text, debounced by
  // spellCheckTimer so a fast typist sends one request, not one per
  // keystroke. Called from Composer.qml on every text change.
  function requestSpellCheck(text: string): void {
    root._lastSpellText = text;
    spellCheckTimer.restart();
  }

  // _runSpellCheck sends the debounced spell.check request. A reply for
  // text the composer has since moved on from is dropped, the same
  // staleness guard pasteImage uses above.
  function _runSpellCheck(): void {
    if (!root.service) return;

    const text = root._lastSpellText;
    const isCurrent = Rpc.guard(() => root._lastSpellText, text);
    root.service.request("spell.check", { text: text }, function(error, result) {
      if (!isCurrent()) return;
      root.spellMisspellings = error ? [] : (result.misspellings || []);
    });
  }

  // requestSpellSuggestions asks the helper for replacement words for
  // word, for the picker Composer.qml opens over a flagged word.
  function requestSpellSuggestions(word: string): void {
    if (!root.service) { root.spellSuggestions = []; return; }

    const isCurrent = Rpc.guard(() => word, word);
    root.service.request("spell.suggest", { word: word }, function(error, result) {
      if (!isCurrent()) return;
      root.spellSuggestions = error ? [] : (result.suggestions || []);
    });
  }

  // addSpellingWord adds word to the user's personal dictionary and
  // re-checks the composer's text at once, so its underline clears
  // without waiting for the next edit.
  function addSpellingWord(word: string): void {
    if (!root.service) return;

    root.service.request("spell.addWord", { word: word }, function(error) {
      if (error) return;

      spellCheckTimer.stop();
      root._runSpellCheck();
    });
  }

  // spellCheckTimer debounces requestSpellCheck so a burst of keystrokes
  // produces one spell.check call, not one per keystroke, the same
  // pattern WindowController's messageSearchTimer uses.
  Timer {
    id: spellCheckTimer
    interval: SpellCheck.DEBOUNCE_MS
    onTriggered: root._runSpellCheck()
  }
}
