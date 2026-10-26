// Checks that j/k reach the open chat's own queued "send later"
// messages once they run past the newest loaded message, and that
// Enter, e and d then act on whichever one is highlighted - send now,
// edit and cancel - instead of a message's own open/react/delete,
// through the same remap Panel.routeKey makes (see
// ConversationController.highlightedScheduledId): a report found the
// palette commands for this undiscoverable from the keyboard at all.
// e is checked in full: it loads the queued row's own text into the
// composer with real keyboard focus there, in writing mode, rather
// than jumping straight to the "Send later…" time prompt and leaving
// the text unreachable (another report, since the prompt that way took
// over the keyboard before the text could be changed at all); typing
// changes the draft, Ctrl+L then opens that same prompt to pick a new
// time, and accepting it reschedules the queued message with both the
// new text and the new time; Esc instead cancels and leaves it queued
// unchanged. Deciding the hint text itself is ui/lib/ScheduledAction.js's
// own job, proven in tests/unit/scheduledaction.test.cjs; this only
// checks the real keyboard reaches the right row and the right call.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "ui/controllers"
import "ui/lib/Keymap.js" as Keymap
import "ui/lib/Navigation.js" as Navigation
import "Check.js" as Check

ShellRoot {
  id: root

  QtObject {
    id: service

    property var accounts: []
    property var services: []
    property var uiState: ({})
    property var pendingAuth: null
    property string status: "ready"
    property string detail: ""
    property int unreadTotal: 0
    property var sendScheduledNowCalls: []
    property var cancelScheduledCalls: []
    property var editScheduledCalls: []

    signal event(string name, var data)

    function start(): void {}
    function installHelper(): void {}
    function quit(): void {}

    function request(method: string, params: var, callback: var): void {
      if (method === "messages.list") {
        callback(null, { hasMore: false, messages: [
          { id: "m1", conversationId: "c1", senderId: "s1", senderName: "Alex", text: "hi",
            outgoing: false, status: "received", created: Date.now() }
        ] });
        return;
      }
      if (method === "messages.listScheduled") {
        callback(null, { messages: [
          { id: "s1", conversationId: "c1", text: "first", sendAt: 1000, created: 1 },
          { id: "s2", conversationId: "c1", text: "second", sendAt: 2000, created: 2 },
          { id: "s3", conversationId: "c1", text: "third", sendAt: 3000, created: 3 }
        ] });
        return;
      }
      if (method === "messages.sendScheduledNow") {
        service.sendScheduledNowCalls.push(params.scheduledId);
        callback(null);
        return;
      }
      if (method === "messages.cancelScheduled") {
        service.cancelScheduledCalls.push(params.scheduledId);
        callback(null);
        return;
      }
      if (method === "messages.editScheduled") {
        service.editScheduledCalls.push({ scheduledId: params.scheduledId, text: params.text, sendAt: params.sendAt });
        const original = scheduledController.all.find((m) => m.id === params.scheduledId);
        callback(null, { id: params.scheduledId, conversationId: original ? original.conversationId : "c1",
          text: params.text, sendAt: params.sendAt, created: original ? original.created : 0 });
        return;
      }
      if (method === "conversations.list") {
        callback(null, []);
        return;
      }

      callback(null, {});
    }
  }

  ListController {
    id: listController
    service: service
  }

  ScheduledController {
    id: scheduledController
    service: service
  }

  ConversationController {
    id: conversationController
    service: service
    listController: listController
    composer: composerController
    scheduledController: scheduledController
  }

  ComposerController {
    id: composerController
    service: service
    conversation: conversationController
  }

  ReactionsController {
    id: reactionsController
    service: service
    conversation: conversationController
  }

  WindowController {
    id: windowController
    service: service
    conversationController: conversationController
    composerController: composerController
    reactionsController: reactionsController
    scheduledController: scheduledController
  }

  // Mirrors MessengerLayout.qml's own two-way wiring between the
  // composer's real focus and ComposerController.composeFocused: this
  // test drives real key events, so e loading a queued message's text
  // back in must actually move keyboard focus into the composer, the
  // same as it does in the real app, for the checks below to tell
  // writing mode from scrolling mode.
  Connections {
    target: composerController
    function onComposeFocusRequested() { view.focusComposer() }
    function onLeaveComposeRequested() {
      if (view.composer.input) view.composer.input.focus = false;
      keyArea.forceActiveFocus();
    }
  }

  Connections {
    target: view.composer.input
    function onActiveFocusChanged() {
      composerController.composeFocused = !!view.composer.input && view.composer.input.activeFocus;
    }
  }

  // routeKey mirrors Panel.routeKey's own dispatch, including the
  // Escape burst reset and the scheduled-row remap: see
  // docs/decisions.md for why this exists rather than driving the
  // controllers directly.
  function routeKey(key: int, modifiers: int, text: string): bool {
    const context = Navigation.keyContext({
      confirmOpen: windowController.confirmingClose,
      setupOpen: false,
      viewerOpen: false,
      paletteOpen: windowController.paletteOpen,
      reactionPickerOpen: reactionsController.pickerOpen,
      dialogOpen: false,
      searchFocused: false,
      composeFocused: composerController.composeFocused,
      pane: conversationController.pane
    });
    const action = Keymap.match(context, key, modifiers, text);

    if (action === "escape") listController.resetBurst();

    const isPlainKey = /^[\x20-\x7e]$/.test(text) && (modifiers & Keymap.MOD.Ctrl) === 0 && (modifiers & Keymap.MOD.Alt) === 0;
    if (isPlainKey && (context === "list" || context === "conversation") && listController.guardTyping(action)) {
      return true;
    }

    if (!action) return false;

    const scheduledId = conversationController.highlightedScheduledId;
    const scheduledRemap = { "message.open": "sendNow", "message.react": "edit", "message.delete": "cancel" };
    if (scheduledId && scheduledRemap[action]) {
      windowController.runScheduledActionOn(scheduledId, scheduledRemap[action]);
      return true;
    }

    const controllers = [listController, conversationController, composerController, reactionsController, windowController];
    const owner = controllers.find((c) => c.handles(action));
    if (!owner) return false;

    return owner.run(action) !== false;
  }

  FloatingWindow {
    id: win
    implicitWidth: 500
    implicitHeight: 400
    visible: true

    Item {
      id: keyArea
      anchors.fill: parent
      focus: true

      Keys.onPressed: event => {
        if (root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
      }

      ConversationView {
        id: view
        anchors.fill: parent
        conversation: conversationController.conversation
        messages: conversationController.messages
        annotations: conversationController.annotations
        highlightedId: conversationController.highlightedId
        highlightedScheduledId: conversationController.highlightedScheduledId
        scheduledMessages: scheduledController.forConversation(conversationController.activeId)
        composeEnabled: true
        draft: composerController.draft
        scheduledEditing: composerController.editingScheduled
        routeKey: root.routeKey

        // Mirrors MessengerLayout.qml's own wiring from the composer's
        // real text input back into ComposerController.draft: without
        // it, typing in the real field here would never reach
        // composerController.draft, the same gap that would make the
        // checks below pass for the wrong reason.
        onDraftEdited: text => composerController.setDraft(text)
      }
    }
  }

  TestCase {
    id: t
    when: false
  }

  Stepper {
    id: stepper
    name: "ScheduledHighlight"
    intervalMs: 20
    deadlineMs: 20000
    onTimeout: () => Check.fail("timed out before the checks finished")
    startFn: root.start
  }

  function start(): void {
    conversationController.open({ id: "c1", accountId: "a1", service: "whatsapp", remoteId: "r1",
      kind: "direct", title: "Alex", members: 0, preview: "", muted: false, unread: 0, lastActivity: Date.now() });
    stepper.attempts = 0;
    root.waitForScheduledLoaded();
  }

  // waitForScheduledLoaded holds until the scheduled list has loaded,
  // since ScheduledController's own messages.listScheduled round trip
  // answers on a later turn of the event loop than conversations.list.
  function waitForScheduledLoaded(): void {
    if (scheduledController.all.length !== 3) {
      stepper.attempts++;
      if (stepper.attempts >= 100) return Check.fail("the scheduled list never loaded");
      return stepper.retry(root.waitForScheduledLoaded);
    }

    root.checkHighlightReachesQueuedRows();
  }

  // checkHighlightReachesQueuedRows walks j past the newest message
  // into the queued rows, soonest due first, clamping at the last one,
  // then k walks back out onto the newest message again.
  function checkHighlightReachesQueuedRows(): void {
    if (conversationController.highlightedId !== "m1")
      return Check.fail("opening the conversation did not start on the newest message: got " + conversationController.highlightedId);

    t.keyClick(Qt.Key_J);
    if (conversationController.highlightedId !== "" || conversationController.highlightedScheduledId !== "s1")
      return Check.fail("j past the newest message did not move onto the soonest queued row: id=" + conversationController.highlightedId
        + " scheduledId=" + conversationController.highlightedScheduledId);

    t.keyClick(Qt.Key_J);
    t.keyClick(Qt.Key_J);
    if (conversationController.highlightedScheduledId !== "s3")
      return Check.fail("j did not walk through every queued row to the last one: got " + conversationController.highlightedScheduledId);

    t.keyClick(Qt.Key_J);
    if (conversationController.highlightedScheduledId !== "s3")
      return Check.fail("j past the last queued row moved the highlight: got " + conversationController.highlightedScheduledId);

    t.keyClick(Qt.Key_K);
    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedScheduledId !== "s1")
      return Check.fail("k did not walk back to the soonest queued row: got " + conversationController.highlightedScheduledId);

    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedScheduledId !== "" || conversationController.highlightedId !== "m1")
      return Check.fail("k past the soonest queued row did not return to the newest message: id="
        + conversationController.highlightedId + " scheduledId=" + conversationController.highlightedScheduledId);

    root.checkHighlightedRowShowsHintsAndOutline();
  }

  // checkHighlightedRowShowsHintsAndOutline moves onto s1 and checks its
  // row shows the key-hint text and an accent outline none of the other
  // rows show.
  function checkHighlightedRowShowsHintsAndOutline(): void {
    t.keyClick(Qt.Key_J);
    if (conversationController.highlightedScheduledId !== "s1") return Check.fail("setup: j did not reach s1");

    const hints = Check.texts(view).map((item) => item.text).find((text) => text.includes("send now"));
    if (!hints || hints !== "Enter send now · e edit · d cancel")
      return Check.fail("the highlighted queued row's own hint text is \"" + hints + "\"");

    root.checkEnterSendsNow();
  }

  // checkEnterSendsNow checks Enter on the highlighted s1 row calls
  // messages.sendScheduledNow for it and drops it from the queue,
  // instead of trying to open a message's photo.
  function checkEnterSendsNow(): void {
    t.keyClick(Qt.Key_Return);
    if (JSON.stringify(service.sendScheduledNowCalls) !== '["s1"]')
      return Check.fail("Enter on the highlighted queued row did not send it now: " + JSON.stringify(service.sendScheduledNowCalls));
    if (scheduledController.all.some((m) => m.id === "s1"))
      return Check.fail("s1 is still queued after Enter sent it now");

    root.checkELoadsTextAndFocusesComposer();
  }

  // checkELoadsTextAndFocusesComposer moves onto s2 (the queue has
  // moved up one) and checks e loads its text into the composer with
  // real keyboard focus there, in writing mode, instead of opening the
  // reaction picker or jumping straight to the send-later prompt (the
  // report this fixes: that left the text itself unreachable, since
  // the prompt took over the keyboard at once). Typing further must
  // still reach the composer's own text, exactly like it would after
  // Up or E loaded a sent message back in to edit.
  function checkELoadsTextAndFocusesComposer(): void {
    if (conversationController.highlightedScheduledId !== "s2")
      return Check.fail("setup: the highlight did not settle on s2 once s1 was sent: got " + conversationController.highlightedScheduledId);

    t.keyClick(Qt.Key_E);
    if (reactionsController.pickerOpen) return Check.fail("e opened the reaction picker instead of editing the queued row");
    if (composerController.draft !== "second")
      return Check.fail("e did not load the queued row's text into the composer: draft is \"" + composerController.draft + "\"");
    if (windowController.paletteOpen)
      return Check.fail("e jumped straight to the send-later prompt, leaving the loaded text unreachable: paletteMode is \"" + windowController.paletteMode + "\"");
    if (!composerController.editingScheduled) return Check.fail("e did not mark the queued row as being edited");
    if (!composerController.composeFocused) return Check.fail("e did not move keyboard focus into the composer");

    const input = view.composer.input;
    if (!input || !input.activeFocus) return Check.fail("the composer's own text input did not get real keyboard focus");

    input.cursorPosition = input.text.length;
    t.keyClick(Qt.Key_X);
    if (composerController.draft !== "secondx")
      return Check.fail("typing after e did not reach the composer's own text: draft is \"" + composerController.draft + "\"");

    root.checkEscapeCancelsScheduledEdit();
  }

  // checkEscapeCancelsScheduledEdit checks Esc backs out of editing s2
  // without rescheduling it: the queued message is left exactly as it
  // was, and the composer's draft is restored to whatever it held
  // before e loaded s2's text in (empty, here). The composer itself
  // stays focused after that, the same as cancelling a sent message's
  // own edit does (see Navigation.escapeAction), so a second Esc - the
  // ordinary "leave the composer" step - is what gets j/k back to
  // moving the highlight for checkDCancels below.
  function checkEscapeCancelsScheduledEdit(): void {
    t.keyClick(Qt.Key_Escape);
    if (composerController.editingScheduled) return Check.fail("Esc did not cancel editing the queued row");
    if (composerController.draft !== "")
      return Check.fail("Esc did not restore the draft it overwrote: draft is \"" + composerController.draft + "\"");

    const s2 = scheduledController.all.find((m) => m.id === "s2");
    if (!s2 || s2.text !== "second")
      return Check.fail("Esc changed the queued message instead of leaving it unchanged: " + JSON.stringify(s2));
    if (service.editScheduledCalls.length !== 0)
      return Check.fail("Esc called the service instead of only cancelling locally: " + JSON.stringify(service.editScheduledCalls));

    t.keyClick(Qt.Key_Escape);
    if (composerController.composeFocused) return Check.fail("a second Esc did not leave the composer");

    root.checkDCancels();
  }

  // checkDCancels moves onto s3 and checks d cancels it outright,
  // instead of opening the delete question a message's own d does.
  // Leaving the composer just now reset the highlight back to the
  // newest loaded message (the same as leaving a sent message's own
  // edit does), so this walks j twice: once back onto the soonest
  // queued row, s2, then again onto s3.
  function checkDCancels(): void {
    t.keyClick(Qt.Key_J);
    if (conversationController.highlightedScheduledId !== "s2")
      return Check.fail("setup: j did not land back on s2 after leaving the composer: got " + conversationController.highlightedScheduledId);

    t.keyClick(Qt.Key_J);
    if (conversationController.highlightedScheduledId !== "s3")
      return Check.fail("setup: j did not reach s3: got " + conversationController.highlightedScheduledId);

    t.keyClick(Qt.Key_D);
    if (JSON.stringify(service.cancelScheduledCalls) !== '["s3"]')
      return Check.fail("d on the highlighted queued row did not cancel it: " + JSON.stringify(service.cancelScheduledCalls));
    if (scheduledController.all.some((m) => m.id === "s3"))
      return Check.fail("s3 is still queued after d cancelled it");

    root.checkCtrlLReschedulesEditedText();
  }

  // checkCtrlLReschedulesEditedText is the full round trip the bug
  // report asked for: e loads s2 (the only message left queued, which
  // cancelling s3 above left highlighted again), typing changes its
  // text, Ctrl+L opens the same send-later prompt to pick a new time,
  // and accepting it reschedules s2 with both the new text and the new
  // time, through messages.editScheduled rather than a fresh
  // messages.schedule.
  function checkCtrlLReschedulesEditedText(): void {
    if (conversationController.highlightedScheduledId !== "s2")
      return Check.fail("setup: cancelling s3 did not settle the highlight back on s2: got " + conversationController.highlightedScheduledId);

    t.keyClick(Qt.Key_E);
    if (composerController.draft !== "second")
      return Check.fail("e did not reload s2's text: draft is \"" + composerController.draft + "\"");
    if (windowController.paletteOpen) return Check.fail("e jumped straight to the send-later prompt again");

    const input = view.composer.input;
    input.cursorPosition = input.text.length;
    t.keyClick(Qt.Key_Y);
    if (composerController.draft !== "secondy")
      return Check.fail("typing before rescheduling did not reach the composer's own text: draft is \"" + composerController.draft + "\"");

    t.keyClick(Qt.Key_L, Qt.ControlModifier);
    if (!windowController.paletteOpen || windowController.paletteMode !== "sendLaterCustom")
      return Check.fail("Ctrl+L did not open the send-later prompt: paletteOpen=" + windowController.paletteOpen + " paletteMode=\"" + windowController.paletteMode + "\"");

    t.keyClick(Qt.Key_Return);
    if (windowController.paletteOpen) return Check.fail("Enter did not accept the send-later prompt");
    if (service.editScheduledCalls.length !== 1 || service.editScheduledCalls[0].scheduledId !== "s2" || service.editScheduledCalls[0].text !== "secondy")
      return Check.fail("accepting the prompt did not reschedule s2 with its new text: " + JSON.stringify(service.editScheduledCalls));
    if (composerController.editingScheduled) return Check.fail("accepting the prompt left editingScheduled true");
    if (composerController.draft !== "") return Check.fail("accepting the prompt left the composer holding text: \"" + composerController.draft + "\"");

    const s2 = scheduledController.all.find((m) => m.id === "s2");
    if (!s2 || s2.text !== "secondy")
      return Check.fail("s2's own text was not updated to the edited one: " + JSON.stringify(s2));

    console.log("PASS ScheduledHighlight");
    Qt.exit(0);
  }
}
