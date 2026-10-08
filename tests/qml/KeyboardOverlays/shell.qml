// Drives every floating overlay open and closed with real key events only
// (no mouse click anywhere in this file), then checks a global shortcut
// still works right afterward: a reload of the plugin or a bug in an
// overlay's own Escape handling can leave the window's key router stuck on
// whatever context the overlay used, which silently breaks every shortcut
// until the window is reopened. The overlays: the command palette, the
// reaction picker, the delete question, the in-app photo viewer, account
// setup (opened from the palette, since there is no "?" shortcut of its
// own: the rail's "?" button runs the same palette.commands action),
// account removal, the new-chat dialog (closed both with Escape and by
// choosing a contact), and the close question. Each step polls until its
// condition holds, because several of the paths it drives answer through
// the helper asynchronously.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property string expected: ""
  // _messagesBeforeDelete is the loaded message count checkDeleteConfirm
  // records just before deleting one, for waitForMessageDeleted to
  // compare against.
  property int _messagesBeforeDelete: 0

  property var steps: [
    root.waitForList,
    root.checkPalette,
    root.openAlex,
    root.waitForAlexOpen,
    root.leaveCompose,
    root.waitForHighlightAfterAlex,
    root.checkReactionPicker,
    root.cancelDeleteConfirm,
    root.checkDeleteConfirm,
    root.waitForMessageDeleted,
    root.checkShortcutAfterDelete,
    root.openMum,
    root.waitForMumOpen,
    root.leaveCompose,
    root.waitForHighlightAfterMum,
    root.checkPhotoViewer,
    root.checkAccountSetup,
    root.checkRemoveAccount,
    root.checkCloseQuestion,
    // Last, since starting a conversation with a contact who had none
    // before permanently adds a twelfth row to the list, which the
    // earlier steps' own checkShortcutStillWorks calls assume is still
    // eleven.
    root.checkNewChatDialogEscape,
    root.openNewChatDialog,
    root.waitForContactsLoaded,
    root.checkNewChatDialogAccept,
    root.waitForNewChatOpened
  ];

  // panel is the live Panel.
  function panel(): var {
    return panelLoader.item;
  }

  // listCount is how many rows the conversation list currently shows.
  function listCount(): int {
    return Check.find(root.panel(), "conversationListView").count;
  }

  // anyVisible reports whether item has a visible descendant (or is one
  // itself) named name. Check.find alone is not enough here: every loaded
  // message delegate carries its own "highlightBar" object, only one of
  // which (the highlighted message's) is ever visible, and Check.find
  // returns whichever one it meets first in the tree regardless of that.
  function anyVisible(item: var, name: string): bool {
    if (!item) return false;
    if (item.objectName === name && item.visible) return true;

    const kids = item.data || item.children;
    if (!kids || typeof kids.length !== "number") return false;
    for (let i = 0; i < kids.length; i++) {
      if (root.anyVisible(kids[i], name)) return true;
    }
    return false;
  }

  // checkShortcutStillWorks presses the rail filter shortcuts and checks
  // they still narrow and restore the list: the plainest proof the key
  // router did not get stuck on the overlay that just closed. It is
  // synchronous (no helper round trip), so it needs no polling of its own.
  function checkShortcutStillWorks(afterWhat: string): var {
    t.keyClick(Qt.Key_2, Qt.ControlModifier);
    if (root.listCount() !== 5) return Check.fail("after " + afterWhat + ", Ctrl+2 gave " + root.listCount() + " rows, want 5");

    t.keyClick(Qt.Key_0, Qt.ControlModifier);
    if (root.listCount() !== 11) return Check.fail("after " + afterWhat + ", Ctrl+0 gave " + root.listCount() + " rows, want 11");

    return true;
  }

  // waitForList holds until the fixture's accounts have all connected.
  function waitForList(): var {
    const accounts = helperService.accounts;
    return root.listCount() === 11 && accounts.length === 3 && accounts.every((a) => a.status === "connected");
  }

  // checkPalette opens the command palette with Ctrl+/, closes it with
  // Escape, and checks a global shortcut still works.
  function checkPalette(): var {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    const palette = Check.find(root.panel(), "commandPalette");
    if (!palette || !palette.visible) return Check.fail("Ctrl+/ did not open the command palette");

    t.keyClick(Qt.Key_Escape);
    if (palette.visible) return Check.fail("Escape did not close the command palette");

    return root.checkShortcutStillWorks("closing the command palette");
  }

  // openAlex jumps to Alex Chen, whose history is small, with Ctrl+K.
  function openAlex(): var {
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    for (const ch of "alex") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForAlexOpen holds until Alex Chen's header and messages are loaded.
  function waitForAlexOpen(): var {
    const title = Check.find(root.panel(), "conversationTitle");
    const messages = Check.find(root.panel(), "messageListView");
    return title && title.text === "Alex Chen" && messages && messages.count > 0;
  }

  // leaveCompose presses Escape once: opening a conversation focuses the
  // composer, and the first Escape leaves it, the same step that resets
  // the message highlight to the newest loaded message (see
  // ConversationController.resetHighlight).
  function leaveCompose(): var {
    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // waitForHighlightAfterAlex holds until a message is highlighted.
  function waitForHighlightAfterAlex(): var {
    return root.anyVisible(root.panel(), "highlightBar");
  }

  // checkReactionPicker presses "e" to react to the highlighted message,
  // closes the picker with Escape, and checks a global shortcut still
  // works.
  function checkReactionPicker(): var {
    t.keyClick(Qt.Key_E);
    const picker = Check.find(root.panel(), "reactionPicker");
    if (!picker || !picker.visible) return Check.fail("e did not open the reaction picker");

    t.keyClick(Qt.Key_Escape);
    if (picker.visible) return Check.fail("Escape did not close the reaction picker");

    return root.checkShortcutStillWorks("closing the reaction picker");
  }

  // cancelDeleteConfirm presses "d" to open the delete question for the
  // highlighted message, then "n" to cancel, and checks nothing was
  // removed: the default answer must never delete a message by accident.
  function cancelDeleteConfirm(): var {
    const before = Check.find(root.panel(), "messageListView").count;

    t.keyClick(Qt.Key_D);
    const confirm = Check.find(root.panel(), "deleteConfirm");
    if (!confirm || !confirm.visible) return Check.fail("d did not open the delete question");

    t.keyClick(Qt.Key_N);
    if (confirm.visible) return Check.fail("n did not cancel the delete question");
    if (Check.find(root.panel(), "messageListView").count !== before) {
      return Check.fail("cancelling the delete question removed a message");
    }

    return true;
  }

  // checkDeleteConfirm reopens the delete question with "d" and answers
  // "m" (delete for me), which is always on offer regardless of who sent
  // the highlighted message, recording the loaded count beforehand for
  // waitForMessageDeleted to compare against once the helper answers.
  function checkDeleteConfirm(): var {
    root._messagesBeforeDelete = Check.find(root.panel(), "messageListView").count;

    t.keyClick(Qt.Key_D);
    const confirm = Check.find(root.panel(), "deleteConfirm");
    if (!confirm || !confirm.visible) return Check.fail("d did not reopen the delete question");

    t.keyClick(Qt.Key_M);
    if (confirm.visible) return Check.fail("m did not close the delete question");

    return true;
  }

  // waitForMessageDeleted holds until the deleted message's row is gone
  // from the loaded timeline, the same message.removed path a deletion
  // reported by the service on its own takes.
  function waitForMessageDeleted(): var {
    return Check.find(root.panel(), "messageListView").count === root._messagesBeforeDelete - 1;
  }

  // checkShortcutAfterDelete is checkShortcutStillWorks under the name
  // this flow's own step reads better with.
  function checkShortcutAfterDelete(): var {
    return root.checkShortcutStillWorks("deleting the highlighted message");
  }

  // openMum jumps to Mum, whose newest loaded message is a photo (see
  // backend/internal/connector/fake/scripts.go), with Ctrl+K.
  function openMum(): var {
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    for (const ch of "mum") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForMumOpen holds until Mum's header and messages are loaded.
  function waitForMumOpen(): var {
    const title = Check.find(root.panel(), "conversationTitle");
    const messages = Check.find(root.panel(), "messageListView");
    return title && title.text === "Mum" && messages && messages.count > 0;
  }

  // waitForHighlightAfterMum holds until a message is highlighted.
  function waitForHighlightAfterMum(): var {
    return root.anyVisible(root.panel(), "highlightBar");
  }

  // checkPhotoViewer presses Enter to open the highlighted (newest)
  // message, which is a photo, closes the viewer with Escape, and checks
  // a global shortcut still works.
  function checkPhotoViewer(): var {
    t.keyClick(Qt.Key_Return);
    const viewer = Check.find(root.panel(), "photoViewer");
    if (!viewer || !viewer.visible) return Check.fail("Enter on Mum's highlighted photo did not open the photo viewer");

    t.keyClick(Qt.Key_Escape);
    if (viewer.visible) return Check.fail("Escape did not close the photo viewer");

    return root.checkShortcutStillWorks("closing the photo viewer");
  }

  // checkAccountSetup opens the command palette, runs "Add an account"
  // (there is no key of its own; it is palette-only, same as the rail's
  // own "+" button for a new chat is not what opens this), closes it with
  // Escape, and checks a global shortcut still works.
  function checkAccountSetup(): var {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    for (const ch of "add an account") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);

    const setup = Check.find(root.panel(), "accountSetup");
    if (!setup || !setup.visible) return Check.fail("running \"Add an account\" from the palette did not open account setup");

    t.keyClick(Qt.Key_Escape);
    if (setup.visible) return Check.fail("Escape did not close account setup");

    return root.checkShortcutStillWorks("closing account setup");
  }

  // checkRemoveAccount opens the command palette, runs "Remove an
  // account", closes the question with Escape without removing anything,
  // and checks a global shortcut still works. A real removal is not
  // exercised here: that path, and that a shortcut still works right
  // after an account is actually removed, is covered in
  // tests/qml/Panel's own checkRemoveAccountKeyboard.
  function checkRemoveAccount(): var {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    for (const ch of "remove an account") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);

    const remove = Check.find(root.panel(), "removeAccount");
    if (!remove || !remove.visible) return Check.fail("running \"Remove an account\" from the palette did not open the question");

    t.keyClick(Qt.Key_Escape);
    if (remove.visible) return Check.fail("Escape did not close the removal question");

    return root.checkShortcutStillWorks("cancelling account removal");
  }

  // checkNewChatDialogEscape opens the new-chat dialog directly with
  // Ctrl+N, closes it with Escape without choosing anyone, and checks
  // keyboard focus actually returns to the key area (not just that a
  // Ctrl+ shortcut still works: the search field forwards unhandled keys
  // to the same router regardless of whether it still holds focus, so
  // that alone would not have caught this), then that a global shortcut
  // still works.
  function checkNewChatDialogEscape(): var {
    t.keyClick(Qt.Key_N, Qt.ControlModifier);
    const dialog = Check.find(root.panel(), "newChatDialog");
    if (!dialog || !dialog.visible) return Check.fail("Ctrl+N did not open the new-chat dialog");

    t.keyClick(Qt.Key_Escape);
    if (dialog.visible) return Check.fail("Escape did not close the new-chat dialog");
    if (dialog.searchField.activeFocus) return Check.fail("Escape closed the dialog but left focus on its search field");
    if (!Check.find(root.panel(), "keyArea").activeFocus) return Check.fail("Escape closed the dialog without returning focus to the key area");

    return root.checkShortcutStillWorks("closing the new-chat dialog with Escape");
  }

  // openNewChatDialog reopens the new-chat dialog with Ctrl+N, this time
  // to choose a contact rather than cancelling.
  function openNewChatDialog(): var {
    t.keyClick(Qt.Key_N, Qt.ControlModifier);
    const dialog = Check.find(root.panel(), "newChatDialog");
    if (!dialog || !dialog.visible) return Check.fail("Ctrl+N did not reopen the new-chat dialog");

    return true;
  }

  // waitForContactsLoaded holds until contacts.list has answered, so the
  // next step's Enter picks a real contact instead of nothing.
  function waitForContactsLoaded(): var {
    const dialog = Check.find(root.panel(), "newChatDialog");
    return dialog && dialog.contacts.length > 0;
  }

  // checkNewChatDialogAccept presses Enter on the first loaded contact,
  // which closes the dialog and opens a conversation with them.
  function checkNewChatDialogAccept(): var {
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForNewChatOpened holds until choosing a contact opened its
  // conversation, then checks a global shortcut still works: the bug this
  // guards against left focus nowhere once the dialog closed, so every
  // shortcut went quiet until the window was reopened. Starting a
  // conversation with a contact who had none before adds a twelfth row
  // to the list, so this checks the rail filters directly rather than
  // through checkShortcutStillWorks, which assumes the original eleven.
  function waitForNewChatOpened(): var {
    const dialog = Check.find(root.panel(), "newChatDialog");
    const title = Check.find(root.panel(), "conversationTitle");
    if (!dialog || dialog.visible || !title || title.text.length === 0) return false;

    t.keyClick(Qt.Key_2, Qt.ControlModifier);
    if (root.listCount() !== 5)
      return Check.fail("after choosing a contact in the new-chat dialog, Ctrl+2 gave " + root.listCount() + " rows, want 5");

    t.keyClick(Qt.Key_0, Qt.ControlModifier);
    if (root.listCount() !== 12)
      return Check.fail("after choosing a contact in the new-chat dialog, Ctrl+0 gave " + root.listCount() + " rows, want 12 (the original 11 plus the new conversation)");

    return true;
  }

  // checkCloseQuestion asks to close with Ctrl+W, cancels with Escape
  // rather than choosing Keep or Quit, and checks a global shortcut still
  // works and the window stayed open.
  function checkCloseQuestion(): var {
    t.keyClick(Qt.Key_W, Qt.ControlModifier);
    const question = Check.find(root.panel(), "closeConfirm");
    if (!question || !question.visible) return Check.fail("Ctrl+W did not show the close question");

    t.keyClick(Qt.Key_Escape);
    if (question.visible) return Check.fail("Escape did not cancel the close question");
    if (!Check.find(root.panel(), "panelWindow").visible) return Check.fail("cancelling the close question left the window hidden");

    return root.checkShortcutStillWorks("cancelling the close question");
  }

  // Named so Panel's own service property, inside the Loader, does not
  // shadow it and bind to itself.
  Service {
    id: helperService
  }

  FakeShell {
    id: fakeShell
    panel: root.panel()
    service: helperService
  }

  Loader {
    id: panelLoader

    sourceComponent: Panel {
      service: helperService
      shell: fakeShell
    }
  }

  TestCase {
    id: t

    when: false
  }

  Stepper {
    id: stepper
    name: "KeyboardOverlays"
    steps: root.steps
    deadlineMs: 55000
    startFn: () => { root.panel().open("{}"); stepper.runStep(); }
  }
}
