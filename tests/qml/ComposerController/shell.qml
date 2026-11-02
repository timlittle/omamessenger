// Checks ComposerController.paste and attachFile against a scripted
// service whose media.paste, clipboard.readText or media.attach reply is
// held back until the test fires it by hand: all three answer
// asynchronously, so one started in one conversation must not attach, or
// insert text, in whatever conversation is open by the time the helper
// replies. paste's own plain-text fallback, once media.paste finds no
// image, goes through clipboard.readText and asks the caller to insert
// the result at the cursor (textInsertRequested) rather than ever
// falling back to the real text field's own paste() - see
// docs/decisions.md for why that fallback was removed outright, not
// merely delayed. attachFile is also checked for calling media.attach
// (never setting attachmentPath straight from the picked path) and for
// showing a refused file as an error rather than attaching it. Also
// checks the editing wiring:
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
import "ui/lib/SpellCheck.js" as SpellCheck
import "Check.js" as Check

ShellRoot {
  id: root

  // insertedTexts records every textInsertRequested text, in order.
  property var insertedTexts: []

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

  // fakeConversation stands in for the ConversationController paste
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
      snapshot: () => fakeConversation._timelineMessages,
      lastError: "",
      notice: "",
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

    onTextInsertRequested: text => root.insertedTexts.push(text)
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
  // it succeeds or fails, is dropped outright, including the text
  // fallback's own clipboard.readText call, which must never even
  // start for a conversation no longer open.
  function run(): void {
    composer.paste();
    const sameConversationReply = root.takePending();
    if (!sameConversationReply) {
      Check.fail("paste did not call media.paste");
      return;
    }

    sameConversationReply(null, { path: "/tmp/same.png" });
    if (composer.attachmentPath !== "/tmp/same.png") {
      Check.fail("a reply for the conversation still open did not attach: " + composer.attachmentPath);
      return;
    }

    composer.attachmentPath = "";
    composer.paste();
    const switchedBeforeSuccess = root.takePending();
    fakeConversation.activeId = "b";
    switchedBeforeSuccess(null, { path: "/tmp/leaked.png" });
    if (composer.attachmentPath !== "") {
      Check.fail("a media.paste reply for a closed conversation attached to the one now open: " + composer.attachmentPath);
      return;
    }

    fakeConversation.activeId = "a";
    composer.paste();
    const switchedBeforeError = root.takePending();
    fakeConversation.activeId = "c";
    switchedBeforeError({ code: -1, message: "no image on the clipboard" }, null);
    if (fakeService.pending) {
      Check.fail("a failed media.paste for a closed conversation still started the text fallback in the one now open");
      return;
    }

    fakeConversation.activeId = "a";
    composer.paste();
    const sameConversationError = root.takePending();
    sameConversationError({ code: -1, message: "no image on the clipboard" }, null);
    const textReply = root.takePending();
    if (!textReply) {
      Check.fail("a failed media.paste for the conversation still open did not fall back to clipboard.readText");
      return;
    }
    if (fakeService.lastMethod !== "clipboard.readText") {
      Check.fail("the text fallback called " + fakeService.lastMethod + ", want clipboard.readText");
      return;
    }

    textReply(null, { text: "see you at 6" });
    if (root.insertedTexts.length !== 1 || root.insertedTexts[0] !== "see you at 6") {
      Check.fail("clipboard.readText's reply did not reach textInsertRequested: " + JSON.stringify(root.insertedTexts));
      return;
    }

    fakeConversation.timeline.lastError = "";
    composer.paste();
    root.takePending()({ code: -1, message: "no image on the clipboard" }, null);
    const refusedTextReply = root.takePending();
    refusedTextReply({ code: -32602, message: "there is no text on the clipboard" }, null);
    if (fakeConversation.timeline.lastError === "") {
      Check.fail("a refused clipboard.readText did not report a notice through the open conversation");
      return;
    }
    fakeConversation.timeline.lastError = "";

    if (!root.checkPickAttachFile()) return;
    if (!root.checkAttachFile()) return;
    if (!root.checkEditFromUp()) return;
    if (!root.checkSaveEdit()) return;
    if (!root.checkCancelEdit()) return;
    if (!root.checkTextTruncated()) return;

    console.log("PASS ComposerController");
    Qt.exit(0);
  }

  // checkPickAttachFile drives pickAttachFile, the attach button's
  // primary path: it asks the helper for an attach-purpose files.pick
  // first, and a successful reply's path flows straight into
  // attachFile (media.attach) exactly as a direct call to it would -
  // never attaching the raw picked path itself. A cancelled
  // pick (files.pick answering with no path) calls neither further;
  // one the portal could not show at all (an "unavailable" reason)
  // reports a fixed notice through the open conversation's lastError
  // instead - there is no in-shell dialog of this UI's own to fall
  // back to any more (see docs/decisions.md).
  function checkPickAttachFile(): bool {
    composer.pickAttachFile();
    const pickReply = root.takePending();
    if (!pickReply) return Check.fail("pickAttachFile did not call the service");
    if (fakeService.lastMethod !== "files.pick")
      return Check.fail("pickAttachFile called " + fakeService.lastMethod + ", want files.pick");
    if (fakeService.lastParams.purpose !== "attach")
      return Check.fail("files.pick params = " + JSON.stringify(fakeService.lastParams) + ", want purpose attach");

    pickReply(null, { path: "/portal/vacation.png" });
    const attachReply = root.takePending();
    if (!attachReply) return Check.fail("a successful pick did not call the service again");
    if (fakeService.lastMethod !== "media.attach")
      return Check.fail("a successful pick called " + fakeService.lastMethod + ", want media.attach");
    if (fakeService.lastParams.path !== "/portal/vacation.png")
      return Check.fail("media.attach params = " + JSON.stringify(fakeService.lastParams) + ", want the picked path");

    attachReply(null, { path: "/outgoing/m3-vacation.png", name: "vacation.png", kind: "photo" });
    if (composer.attachmentPath !== "/outgoing/m3-vacation.png")
      return Check.fail("a successful pick's own attach reply did not attach: " + composer.attachmentPath);

    composer.removeAttachment();
    fakeService.lastMethod = "";
    composer.pickAttachFile();
    const cancelledReply = root.takePending();
    cancelledReply(null, {});
    if (fakeService.lastMethod !== "files.pick")
      return Check.fail("a cancelled pick called the service again: " + fakeService.lastMethod);

    fakeConversation.timeline.lastError = "";
    fakeService.lastMethod = "";
    composer.pickAttachFile();
    const unavailableReply = root.takePending();
    unavailableReply({ code: -32603, message: "internal error", data: { reason: "unavailable" } }, null);
    if (fakeService.lastMethod !== "files.pick")
      return Check.fail("an unavailable portal called the service again: " + fakeService.lastMethod);
    if (fakeConversation.timeline.lastError !== "Couldn't open the file picker: no desktop portal")
      return Check.fail("an unavailable portal did not report the fixed notice: " + fakeConversation.timeline.lastError);

    fakeConversation.timeline.lastError = "";
    return true;
  }

  // checkAttachFile drives attachFile: it calls media.attach rather
  // than setting attachmentPath straight from the picked path, a reply
  // for the conversation still open attaches with the kind and name
  // the helper reported, a reply for one that is not is dropped, and a
  // refused file reports the error through the open conversation
  // rather than attaching anything.
  function checkAttachFile(): bool {
    composer.attachFile("/pick/vacation.png");
    const sameConversationReply = root.takePending();
    if (!sameConversationReply) return Check.fail("attachFile did not call the service");
    if (fakeService.lastMethod !== "media.attach")
      return Check.fail("attachFile called " + fakeService.lastMethod + ", want media.attach");
    if (fakeService.lastParams.path !== "/pick/vacation.png")
      return Check.fail("media.attach params = " + JSON.stringify(fakeService.lastParams));

    sameConversationReply(null, { path: "/outgoing/m1-vacation.png", name: "vacation.png", kind: "photo" });
    if (composer.attachmentPath !== "/outgoing/m1-vacation.png" || composer.attachmentName !== "vacation.png" || composer.attachmentKind !== "photo")
      return Check.fail("a reply for the conversation still open did not attach: " +
        composer.attachmentPath + " " + composer.attachmentName + " " + composer.attachmentKind);

    composer.removeAttachment();
    composer.attachFile("/pick/leaked.png");
    const switchedBeforeSuccess = root.takePending();
    fakeConversation.activeId = "d";
    switchedBeforeSuccess(null, { path: "/outgoing/m2-leaked.png", name: "leaked.png", kind: "photo" });
    if (composer.attachmentPath !== "")
      return Check.fail("a media.attach reply for a closed conversation attached to the one now open: " + composer.attachmentPath);

    fakeConversation.activeId = "a";
    fakeConversation.timeline.lastError = "";
    composer.attachFile("/pick/evil.png");
    const refused = root.takePending();
    refused({ code: -32602, message: "the attached image could not be used" }, null);
    if (composer.attachmentPath !== "")
      return Check.fail("a refused file was still attached: " + composer.attachmentPath);
    if (fakeConversation.timeline.lastError !== "The attached image could not be used.")
      return Check.fail("a refused attach did not surface through the open conversation's error: " + fakeConversation.timeline.lastError);

    fakeConversation.timeline.lastError = "";
    return true;
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

  // checkTextTruncated drives textTruncated, what Composer's own paste
  // or type guard fires once it actually cuts the draft: it must turn
  // straight into the open conversation's own notice (the same field
  // WindowController.toastText already shows), naming the real limit
  // with its thousands separator rather than a raw number.
  function checkTextTruncated(): bool {
    fakeConversation.timeline.notice = "";
    composer.textTruncated(SpellCheck.MAX_COMPOSER_TEXT_LENGTH);

    const want = SpellCheck.truncationNoticeText(SpellCheck.MAX_COMPOSER_TEXT_LENGTH);
    if (fakeConversation.timeline.notice !== want)
      return Check.fail("textTruncated set notice to \"" + fakeConversation.timeline.notice + "\", want \"" + want + "\"");

    fakeConversation.timeline.notice = "";
    return true;
  }
}
