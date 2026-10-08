// Drives Alt+Down/Up (chat.next/chat.prev) and Alt+Shift+Down/Up
// (unread.next/unread.prev) with real key events against the real demo
// helper, across every mode they are meant to work from: the plain
// conversation list (nothing open), an open conversation while scrolling,
// an open conversation while writing, and the all-unreads view. Each step
// reads the next or previous row straight from the list's own model
// rather than assuming a fixed order, since the fake accounts sort by
// recent activity and the exact order is not part of this test's
// contract. XDG_DATA_HOME is set by the test runner, so this never
// touches real data.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  Service {
    id: service
  }

  FakeShell {
    id: fakeShell
    panel: panel
    service: service
  }

  Panel {
    id: panel
    service: service
    shell: fakeShell
  }

  TestCase {
    id: t
    when: false
  }

  Stepper {
    id: stepper
    name: "ChatNavigation"
    deadlineMs: 55000
    onTimeout: () => Check.fail("timed out before the checks finished")
    startFn: root.start
  }

  function start(): void {
    panel.open("{}");
    root.waitForConversations();
  }

  function waitForConversations(): void {
    const listView = Check.find(panel, "conversationListView");
    if (listView && listView.count === 11) return root.checkListModeCursorRelative(listView);

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("got " + (listView ? listView.count : "no list view") + " conversations after retrying, want 11");
    stepper.retry(root.waitForConversations);
  }

  // rowAt reads a model row's {id, title} pair by index.
  function rowAt(listView: var, index: int): var {
    const row = listView.model.get(index);
    return { id: row.id, title: row.title };
  }

  // indexOfId finds a row's index by id, scanning the live model so a
  // step never trusts a position recorded before an earlier step ran.
  function indexOfId(listView: var, id: string): int {
    for (let i = 0; i < listView.model.count; i++) {
      if (listView.model.get(i).id === id) return i;
    }
    return -1;
  }

  // nextUnreadFrom mirrors Selection.nextUnread for the test's own
  // expectations: the next row after fromIndex (wrapping) that is unread
  // and not muted.
  function nextUnreadFrom(listView: var, fromIndex: int): var {
    const count = listView.model.count;
    for (let step = 1; step <= count; step++) {
      const row = listView.model.get((fromIndex + step + count) % count);
      if (row.unread > 0 && !row.muted) return { id: row.id, title: row.title };
    }
    return null;
  }

  // checkListModeCursorRelative moves the list cursor down three times
  // with j, then presses Alt+Down: it must open the row right after the
  // cursor, not some row near the top of the list (the bug report this
  // guards against), and land in scroll mode, not writing mode, since
  // nothing was being written before.
  function checkListModeCursorRelative(listView: var): void {
    t.keyClick(Qt.Key_J);
    t.keyClick(Qt.Key_J);
    t.keyClick(Qt.Key_J);

    root._wantAfterDown = root.rowAt(listView, 4);

    t.keyClick(Qt.Key_Down, Qt.AltModifier);
    stepper.attempts = 0;
    root.waitForListModeOpen(listView);
  }

  property var _wantAfterDown: null
  property var _scrollOpenId: null

  function waitForListModeOpen(listView: var): void {
    const title = Check.find(panel, "conversationTitle");
    const composer = Check.find(panel, "composerInput");

    if (title && title.text === root._wantAfterDown.title) return root.checkListModeLandedScrolling(listView, composer);

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("Alt+Down from the list opened \"" + (title ? title.text : "?") + "\", want \""
        + root._wantAfterDown.title + "\" (the row after the cursor, not the wrong one near the top)");
    stepper.retry(() => root.waitForListModeOpen(listView));
  }

  // checkListModeLandedScrolling checks Alt+Down from the list did not
  // drag the user into writing mode, since nothing was being written.
  function checkListModeLandedScrolling(listView: var, composer: var): void {
    if (composer.activeFocus)
      return Check.fail("Alt+Down from the list left the composer focused; it should have opened the chat in scroll mode");

    root._scrollOpenId = root._wantAfterDown.id;
    root.checkScrollModePreserved(listView);
  }

  // checkScrollModePreserved presses Alt+Up while scrolling an open
  // conversation: it must open the previous row and stay in scroll mode,
  // not snap into writing mode just because opening usually focuses the
  // composer.
  function checkScrollModePreserved(listView: var): void {
    const currentIndex = root.indexOfId(listView, root._scrollOpenId);
    const want = root.rowAt(listView, (currentIndex - 1 + listView.model.count) % listView.model.count);

    t.keyClick(Qt.Key_Up, Qt.AltModifier);
    stepper.attempts = 0;
    root.waitForScrollModeOpen(listView, want);
  }

  function waitForScrollModeOpen(listView: var, want: var): void {
    const title = Check.find(panel, "conversationTitle");
    const composer = Check.find(panel, "composerInput");

    if (title && title.text === want.title) {
      if (composer.activeFocus)
        return Check.fail("Alt+Up while scrolling snapped into writing mode instead of staying in scroll mode");
      root._scrollOpenId = want.id;
      return root.checkWritingModePreserved(listView, composer);
    }

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("Alt+Up while scrolling opened \"" + (title ? title.text : "?") + "\", want \"" + want.title + "\"");
    stepper.retry(() => root.waitForScrollModeOpen(listView, want));
  }

  // checkWritingModePreserved focuses the composer with i, then presses
  // Alt+Down: the next chat must open with the composer still focused,
  // the way Ctrl+J already works while writing.
  function checkWritingModePreserved(listView: var, composer: var): void {
    t.keyClick(Qt.Key_I);
    stepper.attempts = 0;
    root.waitForWritingFocus(listView, composer);
  }

  function waitForWritingFocus(listView: var, composer: var): void {
    if (composer.activeFocus) return root.sendAltDownWhileWriting(listView, composer);

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("i never focused the composer");
    stepper.retry(() => root.waitForWritingFocus(listView, composer));
  }

  function sendAltDownWhileWriting(listView: var, composer: var): void {
    const currentIndex = root.indexOfId(listView, root._scrollOpenId);
    const want = root.rowAt(listView, (currentIndex + 1) % listView.model.count);

    t.keyClick(Qt.Key_Down, Qt.AltModifier);
    stepper.attempts = 0;
    root.waitForWritingModeOpen(listView, composer, want);
  }

  function waitForWritingModeOpen(listView: var, composer: var, want: var): void {
    const title = Check.find(panel, "conversationTitle");

    if (title && title.text === want.title) {
      if (!composer.activeFocus)
        return Check.fail("Alt+Down while writing left writing mode instead of keeping it in the next chat");
      root._scrollOpenId = want.id;
      return root.leaveToListMode();
    }

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("Alt+Down while writing opened \"" + (title ? title.text : "?") + "\", want \"" + want.title + "\"");
    stepper.retry(() => root.waitForWritingModeOpen(listView, composer, want));
  }

  // leaveToListMode steps Escape twice: once out of the composer, once to
  // close the open conversation, back to the plain list.
  function leaveToListMode(): void {
    t.keyClick(Qt.Key_Escape);
    stepper.attempts = 0;
    root.waitForComposeLeft();
  }

  function waitForComposeLeft(): void {
    const composer = Check.find(panel, "composerInput");
    if (composer && !composer.activeFocus) {
      t.keyClick(Qt.Key_Escape);
      stepper.attempts = 0;
      return root.waitForConversationClosed();
    }

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("Escape never left the composer");
    stepper.retry(root.waitForComposeLeft);
  }

  function waitForConversationClosed(): void {
    const listView = Check.find(panel, "conversationListView");
    const title = Check.find(panel, "conversationTitle");

    if (title && title.text === "") return root.checkUnreadNextOpensFromListMode(listView);

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("Escape never closed the open conversation");
    stepper.retry(root.waitForConversationClosed);
  }

  // checkUnreadNextOpensFromListMode presses Alt+Shift+Down from the
  // plain list: unread.next must now open the next unread conversation
  // (not just move the list cursor) and land in scroll mode.
  function checkUnreadNextOpensFromListMode(listView: var): void {
    const cursorIndex = root.indexOfId(listView, listPane_selectedId());
    const want = root.nextUnreadFrom(listView, cursorIndex >= 0 ? cursorIndex : -1);
    if (!want) return Check.fail("no unread conversation found to test unread.next against");

    root._wantUnread = want;
    t.keyClick(Qt.Key_Down, Qt.AltModifier | Qt.ShiftModifier);
    stepper.attempts = 0;
    root.waitForUnreadNextOpen();
  }

  // listPane_selectedId reads the list's current cursor id straight off
  // the real ListColumn, the same row j/k left it on.
  function listPane_selectedId(): string {
    const listColumn = Check.find(panel, "listColumn");
    return listColumn ? listColumn.selectedId : "";
  }

  property var _wantUnread: null

  function waitForUnreadNextOpen(): void {
    const title = Check.find(panel, "conversationTitle");
    const composer = Check.find(panel, "composerInput");

    if (title && title.text === root._wantUnread.title) {
      if (composer.activeFocus)
        return Check.fail("Alt+Shift+Down from the list left the composer focused; it should open in scroll mode");
      return root.leaveToUnreadView();
    }

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("Alt+Shift+Down from the list opened \"" + (title ? title.text : "?")
        + "\", want \"" + root._wantUnread.title + "\" (it should open the next unread chat, not just move the cursor)");
    stepper.retry(root.waitForUnreadNextOpen);
  }

  // leaveToUnreadView closes the conversation unread.next just opened,
  // then switches to the all-unreads view (Ctrl+Shift+A) to check
  // chat.next works there too, over that view's own filtered list.
  function leaveToUnreadView(): void {
    t.keyClick(Qt.Key_Escape);
    stepper.attempts = 0;
    root.waitForClosedThenUnreadView();
  }

  function waitForClosedThenUnreadView(): void {
    const title = Check.find(panel, "conversationTitle");
    if (title && title.text !== "") {
      stepper.attempts++;
      if (stepper.attempts >= 100) return Check.fail("Escape never closed the conversation unread.next opened");
      return stepper.retry(root.waitForClosedThenUnreadView);
    }

    t.keyClick(Qt.Key_A, Qt.ControlModifier | Qt.ShiftModifier);
    stepper.attempts = 0;
    root.waitForUnreadView();
  }

  function waitForUnreadView(): void {
    const listColumn = Check.find(panel, "listColumn");
    if (listColumn && listColumn.unreadView) return root.checkChatNextInUnreadView();

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("Ctrl+Shift+A never showed the all-unreads view");
    stepper.retry(root.waitForUnreadView);
  }

  // checkChatNextInUnreadView puts the cursor on the unread view's own
  // first row, then checks Alt+Down moves within that filtered list
  // (not the full one) and still lands in scroll mode.
  function checkChatNextInUnreadView(): void {
    const listView = Check.find(panel, "conversationListView");
    if (listView.count < 2) return Check.fail("the unread view has fewer than two chats to step between");

    t.keyClick("g"); // cursor.top: jump the list cursor to the first row
    root._wantAfterDown = root.rowAt(listView, 1);

    t.keyClick(Qt.Key_Down, Qt.AltModifier);
    stepper.attempts = 0;
    root.waitForUnreadViewChatNext(listView);
  }

  function waitForUnreadViewChatNext(listView: var): void {
    const title = Check.find(panel, "conversationTitle");
    const composer = Check.find(panel, "composerInput");

    if (title && title.text === root._wantAfterDown.title) {
      if (composer.activeFocus)
        return Check.fail("Alt+Down inside the unread view left the composer focused instead of scroll mode");
      console.log("PASS ChatNavigation");
      return Qt.exit(0);
    }

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("Alt+Down inside the unread view opened \"" + (title ? title.text : "?")
        + "\", want \"" + root._wantAfterDown.title + "\"");
    stepper.retry(() => root.waitForUnreadViewChatNext(listView));
  }
}
