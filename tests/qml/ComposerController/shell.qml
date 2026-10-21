// Checks ComposerController.pasteImage against a scripted service whose
// media.paste reply is held back until the test fires it by hand: pasting
// an image answers asynchronously, so a paste started in one conversation
// must not attach, or fall back to a plain paste, in whatever conversation
// is open by the time the helper replies. Also checks the editing wiring:
// compose.editLast (Up in an empty composer) loads the conversation's
// last editable message, saveEdit (Enter while editing) calls
// messages.edit with the right params, and cancelEdit (Esc while
// editing) restores the draft it overwrote. Which message counts as
// "editable" and whether Up means edit at all is Edit.js's own job,
// proven exhaustively in tests/unit/edit.test.cjs; this only checks the
// controller reached it and reacted to what it said.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property int fallbackCount: 0

  // fakeService holds media.paste's callback rather than answering at
  // once, so the test controls exactly when the reply lands relative to
  // the conversation switching underneath it. lastMethod and lastParams
  // record the most recent call, for the editing checks below.
  QtObject {
    id: fakeService

    property var pending: null
    property string lastMethod: ""
    property var lastParams: null
    // uiState stands in for the real Service's own durable state:
    // setDraft reads and writes its drafts map when it persists a draft
    // for a known conversation (see fakeConversation.activeId).
    property var uiState: ({ drafts: {} })

    function request(method: string, params: var, callback: var): void {
      fakeService.lastMethod = method;
      fakeService.lastParams = params;
      fakeService.pending = callback;
    }
  }

  // fakeConversation stands in for the ConversationController pasteImage
  // and the editing checks read the open conversation's id, timeline and
  // applyMessage from. _timelineMessages holds one incoming and one
  // editable outgoing message, the minimum compose.editLast needs to
  // find the right one.
  QtObject {
    id: fakeConversation

    property string activeId: "a"
    property var appliedMessages: []
    property var _timelineMessages: [
      { id: "m-incoming", outgoing: false, remoteId: "r0", media: null, text: "hey" },
      { id: "m-editable", outgoing: true, remoteId: "r1", media: null, text: "original text" }
    ]
    property var timeline: ({
      model: fakeConversation._timelineMessages,
      lastError: "",
      messageById: (id) => fakeConversation._timelineMessages.find((m) => m.id === id) ?? null
    })

    function applyMessage(message: var): void {
      fakeConversation.appliedMessages.push(message);
    }

    // saveUiState stands in for the real ConversationController's own
    // method, which setDraft calls to persist a draft; nothing here
    // reads it back, so recording it is enough.
    function saveUiState(patch: var): void {
      fakeService.uiState = Object.assign({}, fakeService.uiState, patch);
    }
  }

  ComposerController {
    id: composer
    service: fakeService
    conversation: fakeConversation

    onPasteFallbackRequested: root.fallbackCount++
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // takePending returns the service's held-back callback and clears it,
  // so each case starts from a clean slate.
  function takePending(): var {
    const callback = fakeService.pending;
    fakeService.pending = null;
    return callback;
  }

  // run drives every case in turn: a reply for the conversation that is
  // still open attaches normally; a reply for one that is not, whether
  // it succeeds or fails, is dropped outright.
  function run(): void {
    composer.pasteImage();
    const sameConversationReply = root.takePending();
    if (!sameConversationReply) {
      Check.fail("pasteImage did not call media.paste");
      return;
    }

    sameConversationReply(null, { path: "/tmp/same.png" });
    if (composer.attachmentPath !== "/tmp/same.png") {
      Check.fail("a reply for the conversation still open did not attach: " + composer.attachmentPath);
      return;
    }

    composer.attachmentPath = "";
    composer.pasteImage();
    const switchedBeforeSuccess = root.takePending();
    fakeConversation.activeId = "b";
    switchedBeforeSuccess(null, { path: "/tmp/leaked.png" });
    if (composer.attachmentPath !== "") {
      Check.fail("a media.paste reply for a closed conversation attached to the one now open: " + composer.attachmentPath);
      return;
    }

    composer.pasteImage();
    const switchedBeforeError = root.takePending();
    fakeConversation.activeId = "c";
    switchedBeforeError({ code: -1, message: "no image on the clipboard" }, null);
    if (root.fallbackCount !== 0) {
      Check.fail("a failed media.paste for a closed conversation fell back to a plain paste in the one now open");
      return;
    }

    composer.pasteImage();
    const sameConversationError = root.takePending();
    sameConversationError({ code: -1, message: "no image on the clipboard" }, null);
    if (root.fallbackCount !== 1) {
      Check.fail("a failed media.paste for the conversation still open did not fall back to a plain paste");
      return;
    }

    if (!root.checkEditFromUp()) return;
    if (!root.checkSaveEdit()) return;
    if (!root.checkCancelEdit()) return;

    console.log("PASS ComposerController");
    Qt.exit(0);
  }

  // checkEditFromUp drives compose.editLast (Up in an empty composer):
  // with the draft empty, it loads the conversation's one editable
  // message into the draft and enters editing mode.
  function checkEditFromUp(): bool {
    composer.draft = "";
    if (!composer.run("compose.editLast")) return Check.fail("compose.editLast found nothing to do, want the editable message loaded");
    if (!composer.editing) return Check.fail("compose.editLast did not enter editing mode");
    if (composer.draft !== "original text") return Check.fail("compose.editLast did not load the message's text: " + composer.draft);
    return true;
  }

  // checkSaveEdit drives saveEdit, what Enter runs while editing: it
  // calls messages.edit with the edited message's id and the new text,
  // clears editing before the service has even answered, and applies
  // the service's own reply once it does.
  function checkSaveEdit(): bool {
    composer.saveEdit("edited text");
    if (composer.editing) return Check.fail("saveEdit left editing true before the service answered");

    const callback = root.takePending();
    if (!callback) return Check.fail("saveEdit did not call the service");
    if (fakeService.lastMethod !== "messages.edit")
      return Check.fail("saveEdit called " + fakeService.lastMethod + ", want messages.edit");

    const params = fakeService.lastParams;
    if (params.conversationId !== fakeConversation.activeId || params.messageId !== "m-editable" || params.text !== "edited text")
      return Check.fail("messages.edit params = " + JSON.stringify(params));

    callback(null, { id: "m-editable", conversationId: fakeConversation.activeId, text: "edited text" });
    if (fakeConversation.appliedMessages.length !== 1 || fakeConversation.appliedMessages[0].text !== "edited text")
      return Check.fail("saveEdit's reply was not applied: " + JSON.stringify(fakeConversation.appliedMessages));

    return true;
  }

  // checkCancelEdit drives cancelEdit, what Esc runs while editing: it
  // leaves editing mode and restores the draft startEdit overwrote,
  // without calling the service.
  function checkCancelEdit(): bool {
    fakeService.lastMethod = "";
    composer.draft = "";
    if (!composer.run("compose.editLast")) return Check.fail("compose.editLast found nothing to do for the cancel check");
    if (!composer.editing) return Check.fail("not editing before cancelEdit");

    composer.cancelEdit();
    if (composer.editing) return Check.fail("cancelEdit left editing true");
    if (composer.draft !== "") return Check.fail("cancelEdit did not restore the draft it overwrote: " + composer.draft);
    if (fakeService.lastMethod !== "") return Check.fail("cancelEdit called the service: " + fakeService.lastMethod);

    return true;
  }
}
