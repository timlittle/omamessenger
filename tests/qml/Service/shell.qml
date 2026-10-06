// Checks the walking skeleton end to end: the helper reaches ready, the
// demo seeds the right number of conversations, and a sent message is
// reported delivered. Uses the real demo helper, started by Service
// itself; XDG_DATA_HOME is set by the test runner, so this never touches
// real data.
import QtQuick
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property var conversations: []
  property string pendingConversationId: ""
  property int listAttempts: 0

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
        root.sendToMum();
        return;
      }

      root.listAttempts++;
      if (root.listAttempts >= 50)
        return Check.fail("got " + (result ? result.length : 0) + " conversations after retrying, want 11");

      retryTimer.start();
    });
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
      if (name !== "message.updated") return;
      if (data.conversationId !== root.pendingConversationId) return;
      if (data.status === "delivered") root.succeed();
    }
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
