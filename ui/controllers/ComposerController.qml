import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Format.js" as Format

// Owns the composer's state for the open conversation: its draft text,
// the message it is replying to, a pasted or attached file waiting to be
// sent, and whether it holds keyboard focus. conversation supplies the
// open conversation's id and timeline, and the place a draft is saved;
// set it once, from whoever wires the controllers together.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
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
      "message.reply": () => root.startReply(root.conversation.timeline.newestId()),
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
  // still pastes text, same as it always did.
  function pasteImage(): void {
    if (!root.service) return;

    root.service.request("media.paste", {}, function(error, result) {
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
}
