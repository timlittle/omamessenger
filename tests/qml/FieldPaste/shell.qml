// Checks that Ctrl+V and Shift+Insert, pressed with real key events in
// the command palette's search field, the conversation list's search
// field and account setup's api_id field, each go through the helper's
// bounded clipboard.readText - never Qt's own in-process paste, which
// has no size or time limit of its own (see docs/decisions.md). This
// offscreen test has no real Wayland clipboard at all, so the field
// ending up with fakeService's own scripted text, rather than staying
// empty, is itself proof the native path never ran.
//
// routeKey below reproduces only the one piece of Panel.qml's own
// router this needs - Keymap.match resolving "field.paste" and
// pasting into whichever field in win currently holds focus through a
// real ClipboardPaste - not the full controller graph the real
// Panel.qml wires around it; tests/qml/Controllers and
// tests/qml/Flows already exercise that graph end to end.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "ui/controllers"
import "ui/lib/Keymap.js" as Keymap
import "Check.js" as Check

ShellRoot {
  id: root

  // clipboardText is what fakeService's clipboard.readText always
  // answers with: two lines, so each step also proves a single-line
  // field collapses them to one rather than breaking its own layout.
  property string clipboardText: "pasted text\nsecond line"
  // pastedInto is what the single-line fix above should leave in any
  // field this test pastes into.
  readonly property string pastedInto: "pasted text second line"

  // fakeService records the last method it answered and always
  // replies with clipboardText, the same scripted shape
  // ComposerController's own test uses in place of a real helper.
  property var fakeService: QtObject {
    property string lastMethod: ""

    function request(method, params, callback) {
      fakeService.lastMethod = method;
      callback(null, { text: root.clipboardText });
    }
  }

  ClipboardPaste {
    id: clipboardPaste
    service: root.fakeService
  }

  // routeKey matches Ctrl+V/Shift+Insert to field.paste the same way
  // Panel.qml's own router does and pastes into whichever field in
  // win currently holds focus, found the same way Panel.qml finds it
  // (an Item's own Window.activeFocusItem); every other key is left
  // unhandled, since no step here presses one.
  function routeKey(key: int, modifiers: int, text: string): bool {
    const action = Keymap.match("search", key, modifiers, text);
    if (action !== "field.paste") return false;

    clipboardPaste.pasteInto(fieldLoader.Window.activeFocusItem);
    return true;
  }

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 400
    visible: true

    Loader {
      id: fieldLoader
      anchors.fill: parent
    }
  }

  TestCase {
    id: t
    when: false
  }

  Component {
    id: paletteComponent
    CommandPalette {
      open: true
      routeKey: root.routeKey
    }
  }

  Component {
    id: searchComponent
    ListColumn {
      model: []
      routeKey: root.routeKey
      Component.onCompleted: focusSearch()
    }
  }

  Component {
    id: setupComponent
    AccountSetup {
      stage: "credentials"
      routeKey: root.routeKey
    }
  }

  Stepper {
    id: stepper
    name: "Ctrl+V and Shift+Insert paste through clipboard.readText in the palette, search and setup fields"
    startFn: () => { fieldLoader.sourceComponent = paletteComponent; stepper.runStep(); }
    steps: [
      () => !!Check.find(fieldLoader.item, "paletteField")?.activeFocus,
      () => { t.keyClick(Qt.Key_V, Qt.ControlModifier); return true; },
      () => fakeService.lastMethod === "clipboard.readText" || "the palette field never reached clipboard.readText",
      () => Check.find(fieldLoader.item, "paletteField").text === root.pastedInto
        || ("the palette field's text is wrong: " + Check.find(fieldLoader.item, "paletteField").text),

      () => { fakeService.lastMethod = ""; fieldLoader.sourceComponent = searchComponent; return true; },
      () => !!Check.find(fieldLoader.item, "searchField")?.activeFocus,
      () => { t.keyClick(Qt.Key_Insert, Qt.ShiftModifier); return true; },
      () => fakeService.lastMethod === "clipboard.readText" || "the search field never reached clipboard.readText",
      () => Check.find(fieldLoader.item, "searchField").text === root.pastedInto
        || ("the search field's text is wrong: " + Check.find(fieldLoader.item, "searchField").text),

      () => { fakeService.lastMethod = ""; fieldLoader.sourceComponent = setupComponent; return true; },
      () => !!Check.find(fieldLoader.item, "apiIdField")?.activeFocus,
      () => { t.keyClick(Qt.Key_V, Qt.ControlModifier); return true; },
      () => fakeService.lastMethod === "clipboard.readText" || "the setup field never reached clipboard.readText",
      () => Check.find(fieldLoader.item, "apiIdField").text === root.pastedInto
        || ("the setup field's text is wrong: " + Check.find(fieldLoader.item, "apiIdField").text)
    ]
  }
}
