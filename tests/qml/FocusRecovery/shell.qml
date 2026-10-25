// Global shortcuts (Ctrl+K, Ctrl+/) must keep working no matter what
// last held keyboard focus: the report this guards against found them
// doing nothing with no conversation open at all, and with a blocked DM
// open, after a recent change (see docs/decisions.md's "destroy the
// search field and composer input when hidden") put the composer's own
// text input behind a Loader gated on whether there is something to
// compose into, and disabled it outright while the open contact is
// blocked. Disabling or destroying the field that held active focus
// left nothing to hand it back to keyArea, so every key fell on deaf
// ground.
//
// - checkPaletteWithNothingOpen: fresh window, nothing ever focused, Ctrl+K
//   must still open the palette
// - checkPaletteAndCommandsWhileBlocked: open a direct chat, block its
//   contact through the fake helper (bypassing the confirm dialog, which
//   is its own, already-covered journey), then check both Ctrl+K and
//   Ctrl+/ still work
// - checkShortcutsAfterBlockingWhileTyping: open a second direct chat,
//   focus the composer and type, block that contact mid-type, and check
//   a shortcut still works once the composer's own field is disabled out
//   from under real keyboard focus
// XDG_DATA_HOME is set by the test runner, so this never touches real data.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  // jordanId and dentistId are two of the demo accounts' own direct
  // chats, read off the list model once opened, so contacts.block can
  // be aimed at a real conversation id without a UI round trip through
  // the block confirmation dialog, which is already covered elsewhere.
  property string jordanId: ""
  property string dentistId: ""

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
    name: "FocusRecovery"
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
    if (listView && listView.count === 11) return root.checkPaletteWithNothingOpen();

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("got " + (listView ? listView.count : "no list view") + " conversations after retrying, want 11");
    stepper.retry(root.waitForConversations);
  }

  // conversationIdByTitle reads a conversation's id straight off the
  // list's own model, by its title.
  function conversationIdByTitle(title: string): string {
    const model = Check.find(panel, "conversationListView").model;
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).title === title) return model.get(i).id;
    }
    return "";
  }

  // checkPaletteWithNothingOpen presses Ctrl+K with no conversation ever
  // opened in this panel: nothing has taken keyboard focus away from
  // keyArea yet, so this is the baseline every later case must keep
  // matching, not just the blocked-chat cases.
  function checkPaletteWithNothingOpen(): void {
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    stepper.attempts = 0;
    root.waitForPaletteOpenNothingOpen();
  }

  function waitForPaletteOpenNothingOpen(): void {
    const palette = Check.find(panel, "commandPalette");
    if (palette && palette.visible) {
      t.keyClick(Qt.Key_Escape);
      stepper.attempts = 0;
      return root.waitForPaletteClosedNothingOpen();
    }

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("Ctrl+K did nothing with no conversation open: the command palette never opened");
    stepper.retry(root.waitForPaletteOpenNothingOpen);
  }

  function waitForPaletteClosedNothingOpen(): void {
    const palette = Check.find(panel, "commandPalette");
    if (!palette) return root.openJordan();

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("Escape never closed the command palette");
    stepper.retry(root.waitForPaletteClosedNothingOpen);
  }

  // openJordan jumps to a direct chat nothing else in this suite touches,
  // so blocking it cannot interfere with another test's own data.
  function openJordan(): void {
    root.jordanId = root.conversationIdByTitle("Jordan (Manager)");
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    root.type("jordan");
    t.keyClick(Qt.Key_Return);
    stepper.attempts = 0;
    root.waitForJordanOpen();
  }

  function type(text: string): void {
    for (const ch of text) t.keyClick(ch);
  }

  function waitForJordanOpen(): void {
    const title = Check.find(panel, "conversationTitle");
    if (title && title.text === "Jordan (Manager)") return root.blockJordan();

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("Ctrl+K never opened Jordan (Manager)");
    stepper.retry(root.waitForJordanOpen);
  }

  // blockJordan blocks Jordan's contact directly through the helper,
  // the way the UI's own confirm dialog would once accepted: that
  // dialog's own keyboard journey is already covered, so this skips
  // straight to the state this suite cares about, a blocked open chat.
  function blockJordan(): void {
    service.request("contacts.block", { conversationId: root.jordanId }, function() {});
    stepper.attempts = 0;
    root.waitForJordanBlocked();
  }

  function waitForJordanBlocked(): void {
    const banner = Check.find(panel, "blockedBanner");
    if (banner && banner.visible) return root.checkPaletteWhileBlocked();

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("blocking Jordan never showed the composer's blocked banner");
    stepper.retry(root.waitForJordanBlocked);
  }

  // checkPaletteWhileBlocked presses Ctrl+K with a blocked DM open: the
  // composer's own input is disabled (never destroyed) here, the first
  // of the two ways the regression left focus stranded.
  function checkPaletteWhileBlocked(): void {
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    stepper.attempts = 0;
    root.waitForPaletteOpenBlocked();
  }

  function waitForPaletteOpenBlocked(): void {
    const palette = Check.find(panel, "commandPalette");
    if (palette && palette.visible) {
      t.keyClick(Qt.Key_Escape);
      stepper.attempts = 0;
      return root.waitForPaletteClosedBlocked();
    }

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("Ctrl+K did nothing with a blocked DM open: the command palette never opened");
    stepper.retry(root.waitForPaletteOpenBlocked);
  }

  function waitForPaletteClosedBlocked(): void {
    const palette = Check.find(panel, "commandPalette");
    if (!palette) return root.checkCommandsWhileBlocked();

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("Escape never closed the command palette");
    stepper.retry(root.waitForPaletteClosedBlocked);
  }

  // checkCommandsWhileBlocked presses Ctrl+/, the other global shortcut
  // the report named, still with the blocked DM open.
  function checkCommandsWhileBlocked(): void {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    stepper.attempts = 0;
    root.waitForCommandsOpenBlocked();
  }

  function waitForCommandsOpenBlocked(): void {
    const palette = Check.find(panel, "commandPalette");
    if (palette && palette.visible) {
      t.keyClick(Qt.Key_Escape);
      stepper.attempts = 0;
      return root.waitForCommandsClosedBlocked();
    }

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("Ctrl+/ did nothing with a blocked DM open: the command palette never opened");
    stepper.retry(root.waitForCommandsOpenBlocked);
  }

  function waitForCommandsClosedBlocked(): void {
    const palette = Check.find(panel, "commandPalette");
    if (!palette) return root.openDentistAndType();

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("Escape never closed the command palette");
    stepper.retry(root.waitForCommandsClosedBlocked);
  }

  // openDentistAndType opens a second, still-unblocked direct chat,
  // focuses its composer and starts typing: the state the third case
  // needs, a real text input holding real active focus right before it
  // is disabled out from under it.
  function openDentistAndType(): void {
    root.dentistId = root.conversationIdByTitle("Dr. Bartholomew Featherstonehaugh-Wainwright (Dentist)");
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    root.type("dentist");
    t.keyClick(Qt.Key_Return);
    stepper.attempts = 0;
    root.waitForDentistOpen();
  }

  function waitForDentistOpen(): void {
    const title = Check.find(panel, "conversationTitle");
    if (title && title.text.indexOf("Dentist") >= 0) {
      stepper.attempts = 0;
      return root.waitForDentistComposerFocused();
    }

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("Ctrl+K never opened the dentist's chat");
    stepper.retry(root.waitForDentistOpen);
  }

  // waitForDentistComposerFocused holds until opening the chat has
  // auto-focused its composer, the same as opening any other
  // conversation already does, before typing into it: pressing a key
  // to focus it here would risk racing that auto-focus and typing a
  // stray character ahead of the real text.
  function waitForDentistComposerFocused(): void {
    const composer = Check.find(panel, "composerInput");
    if (composer && composer.activeFocus) {
      root.type("are you free");
      stepper.attempts = 0;
      return root.waitForDentistTyped();
    }

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("opening the dentist's chat never focused the composer");
    stepper.retry(root.waitForDentistComposerFocused);
  }

  function waitForDentistTyped(): void {
    const composer = Check.find(panel, "composerInput");
    if (composer && composer.text === "are you free") return root.blockDentistWhileTyping();

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("typing into the dentist's composer never landed");
    stepper.retry(root.waitForDentistTyped);
  }

  // blockDentistWhileTyping blocks the contact whose chat is open and
  // whose composer holds real active focus right now: the moment the
  // report's third case names, blocking the currently open chat while
  // typing.
  function blockDentistWhileTyping(): void {
    service.request("contacts.block", { conversationId: root.dentistId }, function() {});
    stepper.attempts = 0;
    root.waitForDentistBlockedWhileTyping();
  }

  function waitForDentistBlockedWhileTyping(): void {
    const banner = Check.find(panel, "blockedBanner");
    if (banner && banner.visible) return root.checkShortcutAfterBlockingWhileTyping();

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail("blocking the dentist's chat while typing never showed the blocked banner");
    stepper.retry(root.waitForDentistBlockedWhileTyping);
  }

  // checkShortcutAfterBlockingWhileTyping presses Ctrl+/ right after the
  // composer that held real focus was disabled under it: this is the
  // case the bug report called out explicitly as a real-app regression.
  function checkShortcutAfterBlockingWhileTyping(): void {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    stepper.attempts = 0;
    root.waitForCommandsOpenAfterBlockWhileTyping();
  }

  function waitForCommandsOpenAfterBlockWhileTyping(): void {
    const palette = Check.find(panel, "commandPalette");
    if (palette && palette.visible) {
      console.log("PASS FocusRecovery");
      return Qt.exit(0);
    }

    stepper.attempts++;
    if (stepper.attempts >= 100)
      return Check.fail("Ctrl+/ did nothing right after blocking the currently open, currently-typed-into chat");
    stepper.retry(root.waitForCommandsOpenAfterBlockWhileTyping);
  }
}
