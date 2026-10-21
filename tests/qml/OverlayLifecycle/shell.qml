// Guards the fix for a clipboard-exhaustion report: a hidden-but-alive
// editable TextField still reads Wayland clipboard changes on supported
// Qt versions, with no size limit or deadline, so the command palette,
// account setup and the new-chat dialog must be destroyed on close, not
// just hidden (see Overlays.qml). This drives the real Panel with every
// gated overlay closed, then opened one at a time, and checks the whole
// tree for any editable text field other than the conversation list's
// own search field, which exists for as long as the window is open and
// is not part of this fix. The composer's own text input is not in this
// baseline either: with no conversation open, it does not exist at all
// (see Composer.qml's own active and docs/decisions.md) - its own
// lifetime, and the search field's, are checked separately, in
// tests/qml/FieldLifetime.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  // fieldsAlwaysPresent are the only editable fields allowed to exist
  // while every gated overlay is closed and no conversation is open:
  // the conversation list's own search field, always on screen.
  readonly property var fieldsAlwaysPresent: ["searchField"]

  property var steps: [
    root.waitForList,
    root.checkBaseline,
    root.openPalette,
    root.checkPaletteGated,
    root.openAccountSetup,
    root.checkAccountSetupGated,
    root.openNewChatDialog,
    root.checkNewChatDialogGated
  ]

  // panel is the live Panel.
  function panel(): var {
    return panelLoader.item;
  }

  // listCount is how many rows the conversation list currently shows.
  function listCount(): int {
    return Check.find(root.panel(), "conversationListView").count;
  }

  // collectEditableFields walks item and its descendants and appends the
  // objectName of every live field that can be typed into right now:
  // readOnly is false and it has both a cursor and a text value, which
  // rules out a plain Text label (no cursorPosition) and a read-only
  // TextEdit such as a message bubble's body (see
  // MessageBubbleContent.qml), neither of which this fix is about.
  function collectEditableFields(item: var, out: var): void {
    if (!item) return;
    if (typeof item.readOnly === "boolean" && !item.readOnly
        && typeof item.cursorPosition === "number" && typeof item.text === "string") {
      out.push(item.objectName);
    }

    const kids = item.data || item.children;
    if (!kids || typeof kids.length !== "number") return;
    for (let i = 0; i < kids.length; i++) root.collectEditableFields(kids[i], out);
  }

  // editableFieldNames returns the sorted objectNames of every editable
  // field currently in the live tree.
  function editableFieldNames(): var {
    const out = [];
    root.collectEditableFields(root.panel(), out);
    return out.sort();
  }

  // waitForList holds until the fixture's accounts have all connected,
  // the same wait tests/qml/KeyboardOverlays uses, so later steps have a
  // ready window and a known rail to open the palette's commands from.
  function waitForList(): var {
    const accounts = helperService.accounts;
    return root.listCount() === 11 && accounts.length === 3 && accounts.every((a) => a.status === "connected");
  }

  // checkBaseline checks that with every overlay closed, nothing but the
  // composer and the list's own search field can be typed into: proof
  // the scan itself works (it is not vacuously empty) and that nothing
  // else is already wrongly instantiated.
  function checkBaseline(): var {
    const found = root.editableFieldNames();
    if (JSON.stringify(found) !== JSON.stringify(root.fieldsAlwaysPresent.slice().sort())) {
      return Check.fail("editable fields with everything closed: " + JSON.stringify(found)
        + ", want only " + JSON.stringify(root.fieldsAlwaysPresent));
    }
    return true;
  }

  // openPalette presses the keys to open the command palette, once; a
  // separate step polls for the result, so a step the Stepper retries
  // never repeats a keystroke.
  function openPalette(): var {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    return true;
  }

  // checkPaletteGated polls until the command palette is open and has
  // put real keyboard focus in its own field (CommandPalette's own
  // Component.onCompleted, since it is only ever constructed already
  // open), checks its field is the only new one, closes it, and checks
  // the field is gone again rather than merely hidden.
  function checkPaletteGated(): var {
    const field = Check.find(root.panel(), "paletteField");
    if (!field || !field.activeFocus) return false;

    const wantOpen = root.fieldsAlwaysPresent.concat(["paletteField"]).sort();
    const foundOpen = root.editableFieldNames();
    if (JSON.stringify(foundOpen) !== JSON.stringify(wantOpen)) {
      return Check.fail("editable fields with the palette open: " + JSON.stringify(foundOpen) + ", want " + JSON.stringify(wantOpen));
    }

    t.keyClick(Qt.Key_Escape);
    if (Check.find(root.panel(), "commandPalette")) return Check.fail("Escape did not destroy the command palette");

    const foundClosed = root.editableFieldNames();
    if (JSON.stringify(foundClosed) !== JSON.stringify(root.fieldsAlwaysPresent.slice().sort())) {
      return Check.fail("editable fields after closing the palette: " + JSON.stringify(foundClosed));
    }
    return true;
  }

  // openAccountSetup runs the palette's "own API keys" command, which
  // lands straight on the credentials stage, once; a separate step polls
  // for the result.
  function openAccountSetup(): var {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    for (const ch of "own api keys") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // checkAccountSetupGated polls until account setup is open and has
  // focused the credentials step's own field (AccountSetup's own
  // Component.onCompleted, since it is only ever constructed already
  // open), checks its credential, phone and password fields all exist
  // (every stage's field is a static child of the same view, not only
  // the one the current stage shows), closes it, and checks every field
  // is gone again.
  function checkAccountSetupGated(): var {
    const field = Check.find(root.panel(), "apiIdField");
    if (!field || !field.activeFocus) return false;

    const wantOpen = root.fieldsAlwaysPresent.concat(["apiIdField", "apiHashField", "answerField"]).sort();
    const foundOpen = root.editableFieldNames();
    if (JSON.stringify(foundOpen) !== JSON.stringify(wantOpen)) {
      return Check.fail("editable fields with account setup open: " + JSON.stringify(foundOpen) + ", want " + JSON.stringify(wantOpen));
    }

    t.keyClick(Qt.Key_Escape);
    if (Check.find(root.panel(), "accountSetup")) return Check.fail("Escape did not destroy account setup");

    const foundClosed = root.editableFieldNames();
    if (JSON.stringify(foundClosed) !== JSON.stringify(root.fieldsAlwaysPresent.slice().sort())) {
      return Check.fail("editable fields after closing account setup: " + JSON.stringify(foundClosed));
    }
    return true;
  }

  // openNewChatDialog presses the key to open the new-chat dialog, once;
  // a separate step polls for the result.
  function openNewChatDialog(): var {
    t.keyClick(Qt.Key_N, Qt.ControlModifier);
    return true;
  }

  // checkNewChatDialogGated polls until the new-chat dialog is open and
  // has focused its own search field (NewChatDialog's own
  // Component.onCompleted, since it is only ever constructed already
  // open), checks that field is the only new one, closes it, and checks
  // it is gone again.
  function checkNewChatDialogGated(): var {
    const field = Check.find(root.panel(), "newChatSearchField");
    if (!field || !field.activeFocus) return false;

    const wantOpen = root.fieldsAlwaysPresent.concat(["newChatSearchField"]).sort();
    const foundOpen = root.editableFieldNames();
    if (JSON.stringify(foundOpen) !== JSON.stringify(wantOpen)) {
      return Check.fail("editable fields with the new-chat dialog open: " + JSON.stringify(foundOpen) + ", want " + JSON.stringify(wantOpen));
    }

    t.keyClick(Qt.Key_Escape);
    if (Check.find(root.panel(), "newChatDialog")) return Check.fail("Escape did not destroy the new-chat dialog");

    const foundClosed = root.editableFieldNames();
    if (JSON.stringify(foundClosed) !== JSON.stringify(root.fieldsAlwaysPresent.slice().sort())) {
      return Check.fail("editable fields after closing the new-chat dialog: " + JSON.stringify(foundClosed));
    }

    console.log("PASS OverlayLifecycle");
    Qt.exit(0);
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
    name: "OverlayLifecycle"
    steps: root.steps
    deadlineMs: 30000
    startFn: () => { root.panel().open("{}"); stepper.runStep(); }
  }
}
