import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Edit.js" as Edit
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

  // editTarget is the message the composer is about to save new text
  // for: {id}, or null while not editing. Its text is loaded straight
  // into draft (see startEdit), the same way a reply never duplicates
  // its own text storage.
  property var editTarget: null

  // editing is true while editTarget names a message, for the Escape
  // chain and for message.send's save-instead-of-send branch.
  readonly property bool editing: root.editTarget !== null

  // _draftBeforeEdit is the draft startEdit overwrote, restored by
  // cancelEdit so cancelling an edit never loses what was being typed.
  property string _draftBeforeEdit: ""

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

  // run performs action, the only entry point a key router needs. It
  // returns false only for compose.editLast finding nothing to do (the
  // composer holds text, or there is nothing editable), so the key
  // router leaves Up unaccepted and the TextArea's own caret movement
  // runs instead; every other action here always has something to do.
  function run(action: string): bool {
    const handlers = {
      "compose.focus": () => { root.composeFocusRequested(); return true; },
      "message.reply": () => { root.startReply(root.conversation.highlightedId); return true; },
      "message.edit": () => { root.startEdit(root.conversation.highlightedId); return true; },
      "compose.editLast": () => root.startEditFromUp(),
      "message.send": () => { root.submitRequested(); return true; },
      "compose.newline": () => { root.newlineRequested(); return true; },
      "compose.attach": () => { root.pasteImage(); return true; },
      "compose.attachFile": () => { root.attachFileRequested(); return true; }
    };

    const handler = handlers[action];
    return handler ? handler() !== false : true;
  }

  // setDraft stores the composer's text for the open conversation, so it
  // survives the panel being recreated.
  function setDraft(text: string): void {
    root.draft = text;
    if (!root.conversation.activeId) return;

    root.conversation.saveUiState({ drafts: Object.assign({}, root.service.uiState.drafts, { [root.conversation.activeId]: text }) });
  }

  // startReply makes id, a loaded message, the one the composer answers
  // next, cancelling any edit in progress first: the two banners never
  // show together. Nothing changes if it is not loaded.
  function startReply(id: string): void {
    const m = root.conversation.timeline.messageById(id);
    if (!m) return;

    root.cancelEdit();
    root.replyTarget = { id: m.id, senderName: m.outgoing ? "You" : m.senderName, text: Format.singleLine(m.text) };
  }

  // cancelReply clears the composer's reply target without sending.
  function cancelReply(): void {
    root.replyTarget = null;
  }

  // startEdit loads id, a loaded message, into the composer for
  // editing: its text replaces the draft, a banner takes the reply
  // banner's place, and Enter now saves it instead of sending something
  // new (see Composer.qml's onSubmitted wiring in MessengerLayout).
  // Nothing changes if it is not loaded or not something EditMessage
  // would accept (see Edit.isEditable): this account's own text
  // message, with no attachment, that has already reached the service.
  function startEdit(id: string): void {
    const m = root.conversation.timeline.messageById(id);
    if (!Edit.isEditable(m)) return;

    root.cancelReply();
    root._draftBeforeEdit = root.draft;
    root.editTarget = { id: m.id };
    root.setDraft(m.text);
  }

  // startEditFromUp runs compose.editLast: Up in an empty composer loads
  // the conversation's most recently sent editable message, the same as
  // pressing E on it while highlighted (see Edit.upMeansEdit). It
  // returns false when the composer holds text or nothing qualifies, so
  // the key router leaves Up unaccepted and the TextArea's own caret
  // movement runs instead of this. Reads timeline.snapshot(), a plain
  // array, rather than timeline.model, the raw ListModel: Edit.js calls
  // Array.prototype.find on it, which a ListModel does not have.
  function startEditFromUp(): bool {
    if (!root.conversation) return false;

    const messages = root.conversation.timeline.snapshot();
    if (!Edit.upMeansEdit(root.draft, messages)) return false;

    root.startEdit(Edit.lastEditableId(messages));
    return true;
  }

  // cancelEdit clears the composer's edit target and restores whatever
  // draft startEdit overwrote, without saving anything.
  function cancelEdit(): void {
    if (!root.editing) return;

    root.editTarget = null;
    root.setDraft(root._draftBeforeEdit);
    root._draftBeforeEdit = "";
  }

  // saveEdit sends editTarget's new text through messages.edit, clearing
  // the edit target first: Composer.submit() has already emptied the
  // composer by the time this runs, so there is nothing left to restore
  // either way. A refusal surfaces as lastError, the same as a failed
  // send; the message itself is already back to its original text by
  // then, through the helper's own message.updated event, since
  // Commands.EditMessage reverts what it changed before answering.
  function saveEdit(text: string): void {
    if (!root.service || !root.editTarget || !root.conversation) return;

    const id = root.editTarget.id;
    root.editTarget = null;
    root._draftBeforeEdit = "";
    if (!text) return;

    const conversationId = root.conversation.activeId;
    root.service.request("messages.edit", { conversationId: conversationId, messageId: id, text: text }, function(error, result) {
      if (error) { root.conversation.timeline.lastError = Rpc.errorText(error); return; }
      root.conversation.timeline.lastError = "";
      root.conversation.applyMessage(result);
    });
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

  // reset clears the reply, any edit in progress and any attachment,
  // without touching the draft: ConversationController calls this when
  // the open conversation closes or switches to another one.
  function reset(): void {
    root.replyTarget = null;
    root.editTarget = null;
    root._draftBeforeEdit = "";
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
