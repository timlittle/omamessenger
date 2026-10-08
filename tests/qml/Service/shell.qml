// Checks the walking skeleton end to end: the helper reaches ready, the
// demo seeds the right number of conversations, and a sent message is
// reported delivered. Uses the real demo helper, started by Service
// itself; XDG_DATA_HOME is set by the test runner, so this never touches
// real data. Also checks, with a fake shell facade and no helper
// involved, that a notification.clicked event summons this plugin with
// the clicked conversation's id, and is tolerated with no shell at all.
import QtQuick
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property var conversations: []
  property string pendingConversationId: ""
  property int listAttempts: 0

  // notificationClickForwarded records whether Service's event signal
  // relayed a notification.clicked, as the panel would rely on.
  property bool notificationClickForwarded: false

  // fakeShell stands in for the host facade Omarchy injects: a plain
  // object recording what it was asked to summon.
  property var fakeShell: ({
    summonedId: "",
    summonedPayload: "",
    summon: function(id, payloadJson) {
      root.fakeShell.summonedId = id;
      root.fakeShell.summonedPayload = payloadJson;
      return true;
    }
  })

  // succeed reports a pass and stops the test.
  function succeed(): void {
    console.log("PASS Service");
    Qt.exit(0);
  }

  // onReady runs once the helper answers hello. The demo connectors seed
  // their conversations after the server starts accepting requests, so
  // the very first list can still be empty; this retries briefly rather
  // than treating that race as a failure.
  function onReady(): void {
    root.listConversations();
  }

  // listConversations checks for the eleven conversations the README
  // documents, retrying for a few seconds while the demo connectors seed.
  function listConversations(): void {
    service.request("conversations.list", {}, function(error, result) {
      if (error) return Check.fail("conversations.list failed: " + JSON.stringify(error));

      if (Array.isArray(result) && result.length === 11) {
        root.conversations = result;
        root.checkReadReceiptsToggle();
        root.sendToMum();
        return;
      }

      root.listAttempts++;
      if (root.listAttempts >= 50)
        return Check.fail("got " + (result ? result.length : 0) + " conversations after retrying, want 11");

      retryTimer.start();
    });
  }

  // checkNotificationClicked exercises Service's notification.clicked
  // handling directly, needing no helper connection: tolerated with no
  // shell facade at all, then summoning this plugin with the clicked
  // conversation's id once a fake one is set.
  function checkNotificationClicked(): void {
    service._handleEvent("notification.clicked", { conversationId: "conv-9" }); // no shell: must not throw

    service.shell = root.fakeShell;
    service._handleEvent("notification.clicked", { conversationId: "conv-42" });
    service.shell = null;

    if (root.fakeShell.summonedId !== "io.github.omamessenger")
      return Check.fail("summoned plugin id = " + JSON.stringify(root.fakeShell.summonedId)
        + ", want io.github.omamessenger");

    const payload = JSON.parse(root.fakeShell.summonedPayload);
    if (payload.conversationId !== "conv-42")
      return Check.fail("summon payload = " + root.fakeShell.summonedPayload + ", want conversationId conv-42");

    if (!root.notificationClickForwarded)
      return Check.fail("Service did not forward notification.clicked through its event signal");
  }

  // checkReadReceiptsToggle flips incognito read receipts through the
  // real Service and helper round trip: readReceipts starts true, and
  // toggleReadReceipts (the palette's "Toggle read receipts" command)
  // flips Service's own mirror of it both ways. The helper-side effect
  // (MarkRead never reaching a connector) is covered by the app
  // package's own Go tests; this only checks the UI wiring.
  function checkReadReceiptsToggle(): void {
    if (!service.readReceipts) return Check.fail("readReceipts started false, want true by default");

    service.toggleReadReceipts();
    if (service.readReceipts) return Check.fail("toggleReadReceipts did not turn read receipts off");

    service.toggleReadReceipts();
    if (!service.readReceipts) return Check.fail("toggleReadReceipts did not turn read receipts back on");
  }

  // sendToMum sends a message to a conversation that never fails a send
  // attempt, so the test is not flaky.
  function sendToMum(): void {
    const mum = root.conversations.find((c) => c.title === "Mum");
    if (!mum) return Check.fail("no conversation titled Mum in the fake accounts");

    root.pendingConversationId = mum.id;
    service.request("messages.send", { conversationId: mum.id, text: "integration test" }, function(error, result) {
      if (error) return Check.fail("messages.send failed: " + JSON.stringify(error));
      if (!result || result.status === "failed") return Check.fail("message was refused");
    });
  }

  Service {
    id: service

    onStatusChanged: {
      if (service.status === "ready") root.onReady();
    }

    onEvent: function(name, data) {
      if (name === "notification.clicked") root.notificationClickForwarded = true;
      if (name !== "message.updated") return;
      if (data.conversationId !== root.pendingConversationId) return;
      if (data.status === "delivered") root.succeed();
    }
  }

  // Runs once at startup: the notification-click check needs no helper
  // connection, so it does not wait for ready.
  Timer {
    running: true
    interval: 0
    onTriggered: root.checkNotificationClicked()
  }

  // retryTimer re-lists conversations while the demo connectors are still
  // seeding.
  Timer {
    id: retryTimer
    interval: 200
    onTriggered: root.listConversations()
  }

  // A deliberately unreachable deadline: it only fires, and fails the
  // test with a reason, if something above never happens.
  Timer {
    running: true
    interval: 20000
    onTriggered: Check.fail("timed out before the message was delivered")
  }
}
