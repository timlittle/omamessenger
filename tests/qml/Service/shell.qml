// Checks the walking skeleton end to end: the helper reaches ready, the
// demo seeds the right number of conversations, and a sent message is
// reported delivered. Uses the real demo helper, started by Service
// itself; XDG_DATA_HOME is set by the test runner, so this never touches
// real data. Also checks, with a fake shell facade and no helper
// involved, that a notification.clicked event summons this plugin with
// the clicked conversation's id, and is tolerated with no shell at all;
// and, with a fake sound player (so nothing ever reaches the live
// desktop's audio output), that a notification.sound event resolves a
// service-level sound setting through the real helper's sounds.fetch
// and hands its file:// URL to the player, unless a fake
// do-not-disturb check reports "on", in which case nothing plays -
// checked with a fake so this never shells out to the real
// omarchy-shell.
import QtQuick
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property var conversations: []
  property string pendingConversationId: ""
  property int listAttempts: 0
  property int soundCheckAttempts: 0
  property int dndCheckAttempts: 0

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

  // fakeSoundPlayer stands in for Service's real, QtMultimedia-backed
  // player: a plain object recording every URL it was asked to play,
  // so this test never touches a real audio device.
  property var fakeSoundPlayer: ({
    played: [],
    play: function(url) { root.fakeSoundPlayer.played.push(url); }
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
    root.checkNotificationSound();
  }

  // checkNotificationSound drives Service's real notification.sound
  // handling: a service-level sound setting (standing in for the
  // manifest's "WhatsApp notification sound"), a fake player so
  // nothing reaches the live desktop, and a real round trip through
  // sounds.fetch against the demo helper, which validates and copies
  // ping.ogg (this test's own fixture, the same tiny file
  // VoiceNoteController's own tests use) before this ever hands its
  // path to the player.
  function checkNotificationSound(): void {
    service.soundPlayer = root.fakeSoundPlayer;
    service._whatsappSound = String(Qt.resolvedUrl("ping.ogg")).replace("file://", "");
    service._handleEvent("notification.sound", { conversationId: "conv-9", service: "whatsapp", hasCustomSound: false });
    root.pollNotificationSound();
  }

  // pollNotificationSound retries until sounds.fetch's reply has made
  // its way to fakeSoundPlayer.play, or gives up.
  function pollNotificationSound(): void {
    if (root.fakeSoundPlayer.played.length > 0) {
      const url = root.fakeSoundPlayer.played[0];
      if (!url.startsWith("file://")) Check.fail("soundPlayer.play got " + url + ", want a file:// URL");
      else root.checkDoNotDisturbSuppressesSound();
      return;
    }

    root.soundCheckAttempts++;
    if (root.soundCheckAttempts >= 50) return Check.fail("notification.sound never reached soundPlayer.play");

    soundCheckTimer.start();
  }

  // checkDoNotDisturbSuppressesSound replaces the real, shelling-out
  // check with a fake reporting "on", fires another notification.sound
  // event, and confirms nothing more reaches fakeSoundPlayer.play: a
  // do-not-disturb check reporting on must suppress the sound, the
  // same way Omarchy's own notification popups already go quiet
  // under it.
  function checkDoNotDisturbSuppressesSound(): void {
    service.doNotDisturbCheck = function(callback) { callback(true); };
    service._handleEvent("notification.sound", { conversationId: "conv-9", service: "whatsapp", hasCustomSound: false });
    root.pollDoNotDisturb();
  }

  // pollDoNotDisturb waits a few ticks for a (wrongly) in-flight play
  // to land, then fails if one did: fakeSoundPlayer.played must still
  // hold only the one entry from before do-not-disturb was faked on.
  function pollDoNotDisturb(): void {
    root.dndCheckAttempts++;
    if (root.dndCheckAttempts < 5) { dndCheckTimer.start(); return; }

    if (root.fakeSoundPlayer.played.length !== 1)
      Check.fail("a do-not-disturb check reporting \"on\" did not suppress the sound: played = "
        + JSON.stringify(root.fakeSoundPlayer.played));
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

  // checkAccountNotice exercises Service's account.notice handling
  // directly, needing no helper connection: the event's own message
  // lands on accountNotice, and clearAccountNotice (dismissToast's own
  // call, see tests/qml/Controllers) forgets it again.
  function checkAccountNotice(): void {
    service._handleEvent("account.notice", { accountId: "wa", message: "kept on this computer" });
    if (service.accountNotice !== "kept on this computer")
      return Check.fail("accountNotice = " + JSON.stringify(service.accountNotice) + ", want the event's own message");

    service.clearAccountNotice();
    if (service.accountNotice) return Check.fail("clearAccountNotice did not clear it");
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

  // Runs once at startup: the notification-click and account-notice
  // checks need no helper connection, so neither waits for ready.
  Timer {
    running: true
    interval: 0
    onTriggered: {
      root.checkNotificationClicked();
      root.checkAccountNotice();
    }
  }

  // retryTimer re-lists conversations while the demo connectors are still
  // seeding.
  Timer {
    id: retryTimer
    interval: 200
    onTriggered: root.listConversations()
  }

  // soundCheckTimer retries pollNotificationSound while sounds.fetch's
  // round trip is still in flight.
  Timer {
    id: soundCheckTimer
    interval: 100
    onTriggered: root.pollNotificationSound()
  }

  // dndCheckTimer paces pollDoNotDisturb's short wait.
  Timer {
    id: dndCheckTimer
    interval: 50
    onTriggered: root.pollDoNotDisturb()
  }

  // A deliberately unreachable deadline: it only fires, and fails the
  // test with a reason, if something above never happens.
  Timer {
    running: true
    interval: 20000
    onTriggered: Check.fail("timed out before the message was delivered")
  }
}
