import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Attachments.js" as Attachments
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
  // senderName, text}, or null when the user is not replying to
  // anything.
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

  // scheduledEditId is the queued "send later" message the composer is
  // about to replace, once startScheduledEdit has loaded it back in, or
  // "" while not editing one. WindowController.editScheduledMessage sets
  // this (the keyboard's e on a highlighted queued row and the mouse's
  // own "Edit" button both call it, so the two stay identical): it loads
  // the text here first, with the composer focused and writing, instead
  // of jumping straight to the "Send later…" time prompt the way it used
  // to - that left the text itself unreachable, the report this fixes.
  property string scheduledEditId: ""

  // editingScheduled is true while scheduledEditId names a queued
  // message, for the banner, the Escape chain and WindowController's own
  // compose.sendLater and message.send handling.
  readonly property bool editingScheduled: root.scheduledEditId !== ""

  // _draftBeforeScheduledEdit is the draft startScheduledEdit overwrote,
  // restored by cancelScheduledEdit the same way _draftBeforeEdit is for
  // a sent message's own edit.
  property string _draftBeforeScheduledEdit: ""

  // attachmentPath is the file to send with the next message, or "" for
  // none: set by attachFile (the composer's own file picker, through
  // media.attach) or paste (a clipboard image, through media.paste),
  // once either finishes, and read by
  // ConversationController.send(). See Composer.qml for why the caller,
  // not the composer, owns it. It always names a file the helper has
  // already copied into its own outgoing media area and, when it looks
  // like a photo, sanitized with imagesafe: never the raw path the file
  // picker itself returned.
  property string attachmentPath: ""

  // attachmentName is attachmentPath's own display name, from the same
  // round trip: "" for a pasted image, which has no name of its own
  // (Composer.qml then falls back to attachmentPath's own base name).
  property string attachmentName: ""

  // attachmentKind is what media.attach or media.paste found
  // attachmentPath really is - "photo", "video" or "file" - from the
  // file's own bytes, never guessed from its name. Composer.qml shows
  // a decoded preview only when this is "photo".
  property string attachmentKind: ""

  // attachmentSize is attachmentPath's own byte count, from the same
  // media.attach round trip; 0 for a pasted image, which the helper's
  // reply names no size for, or while no attachment is set.
  property real attachmentSize: 0
  // attachmentLimit is the cap attachmentPath was checked against, in
  // bytes; 0 for a pasted image, or while no attachment is set.
  property real attachmentLimit: 0

  // attachmentLimitHint is the open conversation's own account limit,
  // shown as "Up to 2 GB" near the attach button before any file is
  // picked; "" while no conversation is open or its account's limit
  // is not known yet.
  readonly property string attachmentLimitHint: Attachments.attachmentLimitHint(root._openConversationAttachmentLimit())

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

  // textInsertRequested asks the caller to insert text at the
  // composer's cursor, the same way newlineRequested already asks for
  // a line break: paste()'s own plain-text fallback, once an image
  // paste found nothing there, reads the clipboard through the
  // helper's bounded clipboard.readText and asks for it to land here -
  // never through the real text field's own paste(), which would read
  // the clipboard a second time, this time with no size or time limit
  // of its own (see docs/decisions.md).
  signal textInsertRequested(string text)

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
      "compose.attach": () => { root.paste(); return true; },
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
    root.replyTarget = {
      id: m.id, senderName: m.outgoing ? "You" : m.senderName,
      text: Format.singleLine(m.text)
    };
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

  // startScheduledEdit loads a queued "send later" message's text back
  // into the draft so its own words can be changed, cancelling any sent
  // message edit or reply in progress first (the three banners never
  // show together). The composer is left holding focus in writing mode;
  // the caller still has to pick a time afterwards, through Ctrl+L
  // (compose.sendLater), which reads scheduledEditId to reschedule it
  // instead of queuing a fresh message.
  function startScheduledEdit(id: string, text: string): void {
    root.cancelEdit();
    root.cancelReply();
    root._draftBeforeScheduledEdit = root.draft;
    root.scheduledEditId = id;
    root.setDraft(text);
  }

  // cancelScheduledEdit clears the queued message editing in progress
  // and restores whatever draft startScheduledEdit overwrote, leaving
  // the queued message itself unchanged - Esc while editing one.
  function cancelScheduledEdit(): void {
    if (!root.editingScheduled) return;

    root.scheduledEditId = "";
    root.setDraft(root._draftBeforeScheduledEdit);
    root._draftBeforeScheduledEdit = "";
  }

  // clearScheduledEdit drops scheduledEditId once WindowController has
  // actually rescheduled it with the composer's current text, without
  // restoring the draft startScheduledEdit overwrote: the caller is
  // about to clear the composer itself, the same as a normal send does.
  function clearScheduledEdit(): void {
    root.scheduledEditId = "";
    root._draftBeforeScheduledEdit = "";
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

  // attachFile asks the helper to copy path, a file the composer's own
  // file picker chose, into the outgoing media area and sanitize it
  // with imagesafe when it looks like a photo, before the chip ever
  // shows a preview built from it (see docs/decisions.md): the same
  // treatment a pasted image already gets, never a decision made from
  // the file's own extension. media.attach answers asynchronously, so
  // the open conversation is captured when it starts, the same
  // staleness guard paste uses below; an oversized or unsafe
  // image is refused outright and reported the same way a failed send
  // already is. attachmentPath, name and kind are set only once it
  // succeeds, so the chip never shows anything for a refused file.
  function attachFile(path: string): void {
    if (!root.service) return;

    const conversationId = root.conversation ? root.conversation.activeId : "";
    const accountId = root._openConversationAccountId();
    const isCurrent = Rpc.guard(() => root.conversation ? root.conversation.activeId : null, conversationId);
    root.service.request("media.attach", { path: path, accountId: accountId }, function(error, result) {
      if (!isCurrent()) return;
      if (error) { root.conversation.timeline.lastError = Rpc.errorText(error); return; }

      root.attachmentPath = result.path;
      root.attachmentName = result.name;
      root.attachmentKind = result.kind;
      root.attachmentSize = result.size || 0;
      root.attachmentLimit = result.limit || 0;
    });
  }

  // pickAttachFile asks the helper to open the portal's own file
  // chooser for the composer's attach button, out of process from the
  // shared Omarchy shell (see docs/decisions.md): never the
  // in-process GTK dialog that once crashed it, and never a fallback
  // dialog of this UI's own either - Qt's own non-native FileDialog
  // rendered a sender-controlled file name as rich text through its
  // delegate's default AutoText, which a security review found let a
  // markup-named file already on disk run as HTML the moment the
  // dialog merely listed its folder (see docs/decisions.md). A
  // cancelled pick (files.pick answering with no path) does nothing
  // further; one the portal could not show at all reports a fixed
  // notice instead, the same way a failed links.open does.
  function pickAttachFile(): void {
    if (!root.service) return;

    root.service.request("files.pick", { purpose: "attach", title: "Attach a file" }, function(error, result) {
      if (error) {
        root.conversation.timeline.lastError = Rpc.errorReason(error) === "unavailable"
          ? "Couldn't open the file picker: no desktop portal"
          : Rpc.errorText(error);
        return;
      }
      if (!result.path) return; // cancelled

      root.attachFile(result.path);
    });
  }

  // _openConversationAccountId is the open conversation's own account
  // id, read for media.attach's accountId param, or "" while no
  // conversation is open.
  function _openConversationAccountId(): string {
    if (!root.conversation || !root.conversation.conversation) return "";

    return root.conversation.conversation.accountId;
  }

  // _openConversationAttachmentLimit reads the open conversation's own
  // account's attachmentLimit out of service.accounts, for
  // attachmentLimitHint, or 0 while there is no conversation open or
  // its account is not known yet.
  function _openConversationAttachmentLimit(): real {
    const accountId = root._openConversationAccountId();
    if (!accountId || !root.service) return 0;

    const account = (root.service.accounts || []).find((a) => a.id === accountId);
    return account ? (account.attachmentLimit || 0) : 0;
  }

  // removeAttachment clears whatever attachFile or paste set, from
  // the composer's chip, the Escape chain, or once a message carrying
  // it has been handed to the service (see
  // ConversationController.send). Its outgoing copy is not
  // deleted here: with no message yet referring to it, the background
  // sweep that already cleans up an abandoned paste removes it too,
  // once its grace period passes (see cache.Outgoing.Sweep).
  function removeAttachment(): void {
    root.attachmentPath = "";
    root.attachmentName = "";
    root.attachmentKind = "";
    root.attachmentSize = 0;
    root.attachmentLimit = 0;
  }

  // paste asks the helper whether the clipboard holds an image; if it
  // does, it is attached to the next message. Otherwise - no image, or
  // the helper refused it (too large, timed out, or malformed) -
  // Ctrl+V falls back to plain text, read through the same bounded
  // clipboard.readText round trip rather than ever letting the real
  // text field's own paste() read the clipboard itself: an earlier
  // version fell back to exactly that, which read the clipboard's
  // text/plain offer with no size or time limit of its own, even in
  // the case the helper had just refused the same clipboard owner's
  // image for being too large or too slow (see docs/decisions.md).
  // media.paste and clipboard.readText both answer asynchronously, so
  // the open conversation is captured once, before either call; a
  // reply for a conversation no longer open is dropped outright rather
  // than attaching the image or inserting the text into whichever
  // conversation now happens to be open.
  function paste(): void {
    if (!root.service) return;

    const conversationId = root.conversation ? root.conversation.activeId : "";
    const isCurrent = Rpc.guard(() => root.conversation ? root.conversation.activeId : null, conversationId);
    root.service.request("media.paste", {}, function(error, result) {
      if (!isCurrent()) return;
      if (!error) {
        root.attachmentPath = result.path;
        root.attachmentName = "";
        root.attachmentKind = result.kind || "";
        root.attachmentSize = 0;
        root.attachmentLimit = 0;
        return;
      }

      root._pasteText(isCurrent);
    });
  }

  // _pasteText is paste()'s own plain-text fallback, split out to keep
  // paste() within the size guideline: the clipboard's text/plain
  // offer, through the helper's bounded clipboard.readText, inserted
  // at the composer's cursor on success, or a short refusal notice
  // when the clipboard holds no text either.
  function _pasteText(isCurrent: var): void {
    root.service.request("clipboard.readText", {}, function(error, result) {
      if (!isCurrent()) return;
      if (error) { root.conversation.timeline.lastError = Rpc.errorText(error); return; }

      root.textInsertRequested(result.text);
    });
  }

  // reset clears the reply, any edit in progress and any attachment,
  // without touching the draft: ConversationController calls this when
  // the open conversation closes or switches to another one.
  function reset(): void {
    root.replyTarget = null;
    root.editTarget = null;
    root._draftBeforeEdit = "";
    root.scheduledEditId = "";
    root._draftBeforeScheduledEdit = "";
    root.attachmentPath = "";
    root.attachmentName = "";
    root.attachmentKind = "";
    root.attachmentSize = 0;
    root.attachmentLimit = 0;
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
  // staleness guard paste uses above.
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

  // textTruncated shows a brief toast once Composer's own paste-or-type
  // guard actually cut the draft down to limit characters: Composer is
  // a view and never calls the helper or shows anything itself (see
  // qml.md), so this is the one place that turns its signal into the
  // same notice banner WindowController.toastText already shows for
  // every other brief confirmation (see ConversationController.notice).
  // Does nothing with no conversation open, which the signal cannot
  // fire from anyway (Composer.active is false with none open).
  function textTruncated(limit: int): void {
    if (!root.conversation) return;

    root.conversation.timeline.notice = SpellCheck.truncationNoticeText(limit);
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
