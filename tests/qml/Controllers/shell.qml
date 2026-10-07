// Checks the controllers against the real demo helper, started by
// Service itself: the list loads 11 conversations, a rail filter narrows
// it to one service, the cursor moves by id, opening a conversation marks
// it read and loads its messages newest first, a sent message reaches
// delivered, a second loadOlder() while one page is already loading is
// ignored, removing an account closes one of its conversations if it was
// left open and moves the list cursor off it, and the window Escape
// chain dispatches to the right controller. XDG_DATA_HOME is set by the
// test runner, so this never touches real data.
import QtQuick
import Quickshell
import "ui"
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property int pollAttempts: 0
  property int uiStateChanges: 0
  property bool hidden: false
  property var _next: null

  // retry schedules fn to run again shortly, for a condition that depends
  // on a reply from the helper.
  function retry(fn: var): void {
    root._next = fn;
    retryTimer.start();
  }

  // findByTitle returns the row with title from a ListModel, or null.
  function findByTitle(model: var, title: string): var {
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).title === title) return model.get(i);
    }
    return null;
  }

  // findById returns the row with id from a ListModel, or null.
  function findById(model: var, id: string): var {
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).id === id) return model.get(i);
    }
    return null;
  }

  Service {
    id: service

    onUiStateChanged: root.uiStateChanges++
    onStatusChanged: {
      if (service.status === "ready") root.waitForConversations();
    }
  }

  ListController {
    id: listController
    service: service

    onConversationFolded: (id) => conversationController.closeIfOpen(id)
  }

  ConversationController {
    id: conversationController
    service: service
    listController: listController
  }

  DialogController {
    id: dialogController
    service: service

    onOpened: (conversation) => conversationController.open(conversation)
  }

  WindowController {
    id: windowController
    service: service
    listController: listController
    conversationController: conversationController
    dialogController: dialogController

    onHideRequested: root.hidden = true
  }

  AccountController {
    id: accountController
    service: service
  }

  Timer {
    id: retryTimer
    interval: 100
    onTriggered: root._next()
  }

  // A deliberately unreachable deadline: it only fires, and fails the
  // test with a reason, if something above never happens.
  Timer {
    running: true
    interval: 45000
    onTriggered: Check.fail("timed out before the checks finished")
  }

  // waitForConversations retries while the demo connectors are still
  // seeding, the same race the Service test accounts for.
  function waitForConversations(): void {
    if (listController.model.count === 11) return root.checkRailFilter();

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("got " + listController.model.count + " conversations after retrying, want 11");
    root.retry(root.waitForConversations);
  }

  // checkRailFilter narrows the rail to Telegram, checks every visible row
  // is Telegram, then switches back to "all" for the rest of the checks.
  function checkRailFilter(): void {
    // Every row carries every field the list view requires, including a
    // search match the helper leaves out when there is none.
    for (let i = 0; i < listController.model.count; i++) {
      if (typeof listController.model.get(i).match !== "string")
        return Check.fail("row " + i + " has no match field");
    }

    // Saving durable UI state must notify, or the panel never sees it.
    const changesBefore = root.uiStateChanges;
    listController.run("rail.telegram");
    if (root.uiStateChanges === changesBefore)
      return Check.fail("changing the rail did not notify uiState watchers");
    if (service.uiState.railKey !== "service:telegram")
      return Check.fail("uiState.railKey is " + service.uiState.railKey);
    if (listController.model.count !== 5)
      return Check.fail("rail.telegram gave " + listController.model.count + " rows, want 5");

    for (let i = 0; i < listController.model.count; i++) {
      if (listController.model.get(i).service !== "telegram")
        return Check.fail("rail.telegram included a non-Telegram row at index " + i);
    }

    listController.run("rail.all");
    if (listController.model.count !== 11)
      return Check.fail("rail.all gave " + listController.model.count + " rows, want 11");

    root.checkCursor();
  }

  // checkCursor moves the list cursor down then up by id, since the
  // selection is never an index.
  function checkCursor(): void {
    const ids = listController.visibleIds();
    listController.selectId(ids[0]);

    listController.run("cursor.down");
    if (listController.selectedId !== ids[1])
      return Check.fail("cursor.down gave " + listController.selectedId + ", want " + ids[1]);

    listController.run("cursor.up");
    if (listController.selectedId !== ids[0])
      return Check.fail("cursor.up gave " + listController.selectedId + ", want " + ids[0]);

    root.checkOpenConversation();
  }

  // checkOpenConversation opens a conversation with unread messages and
  // starts waiting for it to be marked read and for its messages to load.
  function checkOpenConversation(): void {
    const alex = root.findByTitle(listController.model, "Alex Chen");
    if (!alex) return Check.fail("no conversation titled Alex Chen in the fake accounts");
    if (alex.unread <= 0) return Check.fail("Alex Chen already has no unread messages to clear");

    conversationController.open(alex);
    root.pollAttempts = 0;
    root.waitForOpenConversation(alex.id);
  }

  // waitForOpenConversation holds until the messages finish loading and
  // the list has heard the conversation is read, then checks both.
  function waitForOpenConversation(id: string): void {
    const row = root.findById(listController.model, id);
    const loaded = conversationController.messages.count > 0 && !conversationController.hasMore;

    if (loaded && row && row.unread === 0) {
      if (conversationController.messages.count < 2)
        return Check.fail("Alex Chen loaded " + conversationController.messages.count + " messages, want at least 2");

      const newest = conversationController.messages.get(0).created;
      const oldest = conversationController.messages.get(conversationController.messages.count - 1).created;
      if (newest < oldest) return Check.fail("messages are not newest first: index 0 is older than the last index");

      return root.checkLoadOlder();
    }

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("Alex Chen never finished opening: loaded=" + loaded + " unread=" + (row ? row.unread : "?"));
    root.retry(() => root.waitForOpenConversation(id));
  }

  // checkLoadOlder opens a conversation with more than one page of
  // messages, then calls loadOlder() twice back to back: the guard inside
  // the controller is checked before either request is sent, so the
  // second call does nothing and only one page is ever fetched.
  function checkLoadOlder(): void {
    const omarchy = root.findByTitle(listController.model, "Omarchy Users");
    if (!omarchy) return Check.fail("no conversation titled Omarchy Users in the fake accounts");

    conversationController.open(omarchy);
    root.pollAttempts = 0;
    root.waitForFirstPage();
  }

  // waitForFirstPage holds for the initial 50-message page, then fires
  // loadOlder() twice in the same tick before either can reply.
  function waitForFirstPage(): void {
    if (conversationController.messages.count === 50 && conversationController.hasMore) {
      conversationController.loadOlder();
      conversationController.loadOlder();
      root.pollAttempts = 0;
      return root.waitForSecondPage();
    }

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("Omarchy Users never finished its first page, has " + conversationController.messages.count);
    root.retry(root.waitForFirstPage);
  }

  // waitForSecondPage holds for the one older page the guard allowed
  // through, and fails if a second request slipped past it.
  function waitForSecondPage(): void {
    if (conversationController.messages.count === 100) return root.checkSend();
    if (conversationController.messages.count > 100)
      return Check.fail("loadOlder's guard let two requests through: got " + conversationController.messages.count + " messages, want 100");

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("loadOlder never finished, has " + conversationController.messages.count);
    root.retry(root.waitForSecondPage);
  }

  // checkSend opens a conversation that never fails a send, so the test
  // is not flaky, sends a message and waits for it to be delivered.
  function checkSend(): void {
    const mum = root.findByTitle(listController.model, "Mum");
    if (!mum) return Check.fail("no conversation titled Mum in the fake accounts");

    conversationController.open(mum);
    conversationController.send("integration test", "");
    root.pollAttempts = 0;
    root.waitForDelivered();
  }

  // waitForDelivered holds until the sent message's status reaches
  // delivered through a message.updated event.
  function waitForDelivered(): void {
    for (let i = 0; i < conversationController.messages.count; i++) {
      const m = conversationController.messages.get(i);
      if (m.text === "integration test" && m.status === "delivered") return root.checkRemoveAccountClosesOpenConversation();
    }

    root.pollAttempts++;
    if (root.pollAttempts >= 100) return Check.fail("sent message never reached delivered");
    root.retry(root.waitForDelivered);
  }

  // checkRemoveAccountClosesOpenConversation opens a conversation that
  // belongs to an account not named anywhere else in this test, removes
  // that account, and starts waiting for its conversations to leave the
  // list.
  function checkRemoveAccountClosesOpenConversation(): void {
    const platformTeam = root.findByTitle(listController.model, "Platform Team");
    if (!platformTeam) return Check.fail("no conversation titled Platform Team in the fake accounts");

    conversationController.open(platformTeam);
    if (conversationController.activeId !== platformTeam.id) return Check.fail("Platform Team did not open");

    accountController.remove("tg-work");
    root.pollAttempts = 0;
    root.waitForAccountRemoved(platformTeam.id);
  }

  // waitForAccountRemoved holds until the removed account's conversations
  // are gone from the list, then checks the open conversation closed
  // instead of lingering on a deleted account, with the list cursor
  // moved off it too rather than left dangling on a row that no longer
  // exists.
  function waitForAccountRemoved(removedId: string): void {
    if (listController.all.some((c) => c.accountId === "tg-work")) {
      root.pollAttempts++;
      if (root.pollAttempts >= 100) return Check.fail("tg-work was never removed from the list");
      return root.retry(() => root.waitForAccountRemoved(removedId));
    }

    if (conversationController.activeId !== "" || conversationController.pane !== "list")
      return Check.fail("removing the account left the conversation open: activeId=" + conversationController.activeId);
    if (listController.selectedId === removedId)
      return Check.fail("the list cursor is still on a conversation from the removed account");

    root.checkEscapeChain();
  }

  // checkEscapeChain walks every step Navigation.escapeAction defines,
  // checking the owning controller undid exactly the innermost thing.
  function checkEscapeChain(): void {
    windowController.run("palette.commands");
    windowController.run("escape");
    if (windowController.paletteOpen) return Check.fail("escape did not close the command palette");

    dialogController.open = true;
    windowController.run("escape");
    if (dialogController.open) return Check.fail("escape did not close the dialog");

    listController.searchFocused = true;
    listController.setQuery("abc");
    windowController.run("escape");
    if (listController.query !== "") return Check.fail("escape did not clear a non-empty search query first");
    if (!listController.searchFocused) return Check.fail("escape left the search field after only clearing the query");

    windowController.run("escape");
    if (listController.searchFocused) return Check.fail("escape did not leave the search field once the query was already clear");

    const mum = root.findByTitle(listController.model, "Mum");
    conversationController.open(mum);
    windowController.run("escape");
    if (conversationController.activeId !== "") return Check.fail("escape did not close the open conversation");

    // With nothing left to undo, Escape asks before closing; a second
    // Escape cancels, and choosing to keep it in the background hides it.
    windowController.run("escape");
    if (root.hidden || !windowController.confirmingClose)
      return Check.fail("escape did not ask before closing once nothing else was left to undo");
    windowController.run("escape");
    if (windowController.confirmingClose) return Check.fail("escape did not cancel the close question");
    windowController.run("escape");
    windowController.keepInBackground();
    if (!root.hidden) return Check.fail("keeping it in the background did not hide the window");

    console.log("PASS Controllers");
    Qt.exit(0);
  }
}
