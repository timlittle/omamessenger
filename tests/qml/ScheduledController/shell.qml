// Checks ScheduledController's wiring to messages.schedule,
// .listScheduled, .editScheduled, .cancelScheduled and
// .sendScheduledNow, and that it stays in step with
// scheduledMessage.* events, against a scripted fake service whose own
// reply is held back until the test fires it by hand. Deciding how a
// time parses or how it is labelled belongs to ui/lib (Snooze.js,
// Format.js, Palette.js), already proven in node tests; this only
// checks the controller reached the right method with the right
// params and applied what came back.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property var scheduleError: undefined

  // fakeService holds its own reply back until the test asks for it,
  // the same scripted pattern tests/qml/ComposerController already uses.
  QtObject {
    id: fakeService

    property string status: "starting"
    property var pending: null
    property string lastMethod: ""
    property var lastParams: null

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      fakeService.lastMethod = method;
      fakeService.lastParams = params;
      fakeService.pending = callback;
    }
  }

  ScheduledController {
    id: scheduled
    service: fakeService
  }

  // alreadyReadyService starts "ready" rather than transitioning into
  // it: the state a fresh Panel's controllers are built into every
  // time Omarchy destroys and recreates the panel on hide, since the
  // helper itself (owned by Service.qml) was already running. A
  // controller that only loads on a status *change* would stay
  // permanently empty here, hiding every message queued before this
  // particular panel instance existed - the report's "no way to see
  // scheduled messages" after reopening the window.
  QtObject {
    id: alreadyReadyService

    property string status: "ready"
    property var pending: null
    property string lastMethod: ""

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      alreadyReadyService.lastMethod = method;
      alreadyReadyService.pending = callback;
    }
  }

  ScheduledController {
    id: scheduledAlreadyReady
    service: alreadyReadyService
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // takePending returns the service's held-back callback and clears it.
  function takePending(): var {
    const callback = fakeService.pending;
    fakeService.pending = null;
    return callback;
  }

  function run(): void {
    if (!root.checkLoadsAlreadyReadyService()) return;

    // Becoming ready loads the full list at once.
    fakeService.status = "ready";
    const loadCallback = root.takePending();
    if (!loadCallback) { Check.fail("becoming ready did not call the service"); return; }
    if (fakeService.lastMethod !== "messages.listScheduled") {
      Check.fail("becoming ready called " + fakeService.lastMethod + ", want messages.listScheduled");
      return;
    }

    loadCallback(null, { messages: [
      { id: "s1", conversationId: "a", text: "see you soon", sendAt: 2000, created: 1 },
      { id: "s2", conversationId: "b", text: "happy birthday", sendAt: 1000, created: 1 }
    ] });
    if (scheduled.all.length !== 2) { Check.fail("load did not populate all: " + JSON.stringify(scheduled.all)); return; }

    if (!root.checkForConversation()) return;
    if (!root.checkSchedule()) return;
    if (!root.checkEdit()) return;
    if (!root.checkCancel()) return;
    if (!root.checkSendNow()) return;
    if (!root.checkEvents()) return;

    console.log("PASS ScheduledController");
    Qt.exit(0);
  }

  // checkLoadsAlreadyReadyService confirms a controller built against a
  // service that is already "ready" (never changing into it) still
  // loads the queued list, rather than waiting forever for a status
  // change that will never come.
  function checkLoadsAlreadyReadyService(): bool {
    if (alreadyReadyService.lastMethod !== "messages.listScheduled") {
      Check.fail("a controller built against an already-ready service called " + alreadyReadyService.lastMethod + ", want messages.listScheduled");
      return false;
    }

    alreadyReadyService.pending(null, { messages: [{ id: "r1", conversationId: "a", text: "already queued", sendAt: 9000, created: 1 }] });
    if (scheduledAlreadyReady.all.length !== 1) {
      Check.fail("an already-ready service's reply was not applied: " + JSON.stringify(scheduledAlreadyReady.all));
      return false;
    }
    return true;
  }

  // checkForConversation confirms forConversation filters by id.
  function checkForConversation(): bool {
    const forA = scheduled.forConversation("a");
    if (forA.length !== 1 || forA[0].id !== "s1") {
      Check.fail("forConversation(a) = " + JSON.stringify(forA));
      return false;
    }
    return true;
  }

  // checkSchedule drives schedule(): it calls messages.schedule with
  // the right params and adds the reply to all once it answers.
  function checkSchedule(): bool {
    scheduled.schedule("c", "new message", 3000, (error) => { root.scheduleError = error; });
    const callback = root.takePending();
    if (!callback) { Check.fail("schedule did not call the service"); return false; }
    if (fakeService.lastMethod !== "messages.schedule") {
      Check.fail("schedule called " + fakeService.lastMethod + ", want messages.schedule");
      return false;
    }
    const params = fakeService.lastParams;
    if (params.conversationId !== "c" || params.text !== "new message" || params.sendAt !== 3000) {
      Check.fail("messages.schedule params = " + JSON.stringify(params));
      return false;
    }

    callback(null, { id: "s3", conversationId: "c", text: "new message", sendAt: 3000, created: 2 });
    if (scheduled.all.length !== 3) { Check.fail("schedule's reply was not added: " + JSON.stringify(scheduled.all)); return false; }
    if (root.scheduleError !== null) { Check.fail("schedule reported an error on success: " + root.scheduleError); return false; }
    return true;
  }

  // checkEdit drives edit(): it calls messages.editScheduled naming
  // the queued message, and replaces its row in all with the reply.
  function checkEdit(): bool {
    scheduled.edit("s3", "edited message", 4000, () => {});
    const callback = root.takePending();
    if (!callback) { Check.fail("edit did not call the service"); return false; }
    if (fakeService.lastMethod !== "messages.editScheduled") {
      Check.fail("edit called " + fakeService.lastMethod + ", want messages.editScheduled");
      return false;
    }
    if (fakeService.lastParams.scheduledId !== "s3") {
      Check.fail("messages.editScheduled params = " + JSON.stringify(fakeService.lastParams));
      return false;
    }

    callback(null, { id: "s3", conversationId: "c", text: "edited message", sendAt: 4000, created: 2 });
    const edited = scheduled.all.find((m) => m.id === "s3");
    if (!edited || edited.text !== "edited message" || edited.sendAt !== 4000) {
      Check.fail("edit's reply was not applied: " + JSON.stringify(scheduled.all));
      return false;
    }
    if (scheduled.all.length !== 3) { Check.fail("edit changed how many rows are queued: " + scheduled.all.length); return false; }
    return true;
  }

  // checkCancel drives cancel(): it calls messages.cancelScheduled and
  // drops the row from all once it answers.
  function checkCancel(): bool {
    scheduled.cancel("s2");
    const callback = root.takePending();
    if (!callback) { Check.fail("cancel did not call the service"); return false; }
    if (fakeService.lastMethod !== "messages.cancelScheduled") {
      Check.fail("cancel called " + fakeService.lastMethod + ", want messages.cancelScheduled");
      return false;
    }
    if (fakeService.lastParams.scheduledId !== "s2") {
      Check.fail("messages.cancelScheduled params = " + JSON.stringify(fakeService.lastParams));
      return false;
    }

    callback(null);
    if (scheduled.all.some((m) => m.id === "s2")) { Check.fail("cancel did not remove s2: " + JSON.stringify(scheduled.all)); return false; }
    return true;
  }

  // checkSendNow drives sendNow(): it calls messages.sendScheduledNow
  // and drops the row from all once it answers, the same as cancel.
  function checkSendNow(): bool {
    scheduled.sendNow("s3");
    const callback = root.takePending();
    if (!callback) { Check.fail("sendNow did not call the service"); return false; }
    if (fakeService.lastMethod !== "messages.sendScheduledNow") {
      Check.fail("sendNow called " + fakeService.lastMethod + ", want messages.sendScheduledNow");
      return false;
    }
    if (fakeService.lastParams.scheduledId !== "s3") {
      Check.fail("messages.sendScheduledNow params = " + JSON.stringify(fakeService.lastParams));
      return false;
    }

    callback(null, { id: "m1", conversationId: "c", text: "edited message", outgoing: true, status: "pending" });
    if (scheduled.all.some((m) => m.id === "s3")) { Check.fail("sendNow did not remove s3: " + JSON.stringify(scheduled.all)); return false; }
    if (scheduled.all.length !== 1) { Check.fail("only s1 should be left queued: " + JSON.stringify(scheduled.all)); return false; }
    return true;
  }

  // checkEvents confirms the controller reacts to scheduledMessage.*
  // events the same way it reacts to its own calls' own replies, for a
  // queued message another client (or the sender claiming it) changed.
  function checkEvents(): bool {
    fakeService.event("scheduledMessage.added", { id: "s4", conversationId: "d", text: "from elsewhere", sendAt: 5000, created: 3 });
    if (!scheduled.all.some((m) => m.id === "s4")) { Check.fail("scheduledMessage.added was not applied: " + JSON.stringify(scheduled.all)); return false; }

    fakeService.event("scheduledMessage.updated", { id: "s4", conversationId: "d", text: "changed elsewhere", sendAt: 6000, created: 3 });
    const updated = scheduled.all.find((m) => m.id === "s4");
    if (!updated || updated.text !== "changed elsewhere") { Check.fail("scheduledMessage.updated was not applied: " + JSON.stringify(scheduled.all)); return false; }

    fakeService.event("scheduledMessage.removed", { conversationId: "d", scheduledId: "s4" });
    if (scheduled.all.some((m) => m.id === "s4")) { Check.fail("scheduledMessage.removed was not applied: " + JSON.stringify(scheduled.all)); return false; }

    return true;
  }
}
