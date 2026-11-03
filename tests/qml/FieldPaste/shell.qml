// Checks that Ctrl+V and Shift+Insert, pressed with real key events in
// the command palette's search field, the conversation list's search
// field and account setup's api_id field, each go through the helper's
// bounded clipboard.readText - never Qt's own in-process paste, which
// has no size or time limit of its own (see docs/decisions.md). This
// offscreen test has no real Wayland clipboard at all, so the field
// ending up with fakeService's own scripted text, rather than staying
// empty, is itself proof the native path never ran.
//
// A follow-up review found Qt's own paste shortcut is bigger than
// those two keys (QKeySequence::StandardKey::Paste also covers
// Ctrl+Shift+Insert and, on some platform themes, the dedicated Paste
// key), and that keys.conf rebinding field.paste off Ctrl+V would let
// even that key fall through. This also sends Ctrl+Shift+Insert into
// the palette field, Qt.Key_Paste into the search field, and Ctrl+V
// with field.paste rebound away from it into the setup field, each
// still expected to reach clipboard.readText - proving the field's own
// paste-key interception, not the binding lookup, is what catches
// them.
//
// A later review found Qt's own event.matches(StandardKey.Paste) unfit
// for that interception: Qt resolves it from the desktop's active
// platform theme, which a developer's own session sets and a bare CI
// container never does, so Ctrl+Shift+Insert matched on one and not
// the other. Every field now checks ui/lib/Paste.js's own
// isPasteKey(event.key, event.modifiers) instead - a fixed list of
// every key some keyboard scheme maps to Paste, plus the dedicated
// Paste key - which answers the same way regardless of the platform
// theme (see docs/decisions.md).
//
// routeKey below reproduces only the one piece of Panel.qml's own
// router this needs - Keymap.match resolving "field.paste" (or, for
// nativePaste, skipping that lookup entirely the way Panel.qml's own
// routeKey now does) and pasting into whichever field in win currently
// holds focus through a real ClipboardPaste - not the full controller
// graph the real Panel.qml wires around it; tests/qml/Controllers and
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

  // bindings lets the setup field's own step rebind field.paste away
  // from Ctrl+V and Shift+Insert entirely, proving the paste still
  // reaches clipboard.readText through the field's own isPasteKey
  // interception rather than this binding lookup; every other step
  // leaves it at the real defaults.
  property var bindings: Keymap.BINDINGS

  // reboundBindings has no Ctrl+V/Shift+Insert field.paste binding at
  // all, the shape keys.conf gives it once the user moves that action
  // elsewhere.
  property var reboundBindings: [
    { action: "field.paste", keys: ["Ctrl+Shift+Z"], contexts: ["global"], label: "Paste" }
  ]

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
  // unhandled, since no step here presses one. nativePaste, set only
  // by a field that already caught Qt's own paste shortcut itself,
  // skips the binding lookup entirely - the same way Panel.qml's own
  // routeKey does - so the setup field's rebound-bindings step still
  // pastes.
  function routeKey(key: int, modifiers: int, text: string, nativePaste: bool): bool {
    if (nativePaste) {
      clipboardPaste.pasteInto(fieldLoader.Window.activeFocusItem);
      return true;
    }

    const action = Keymap.match("search", key, modifiers, text, root.bindings);
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
    name: "Ctrl+V, Shift+Insert, Ctrl+Shift+Insert, Key_Paste and a rebound Ctrl+V all paste through clipboard.readText in the palette, search and setup fields"
    startFn: () => { fieldLoader.sourceComponent = paletteComponent; stepper.runStep(); }
    steps: [
      () => !!Check.find(fieldLoader.item, "paletteField")?.activeFocus,
      () => { t.keyClick(Qt.Key_V, Qt.ControlModifier); return true; },
      () => fakeService.lastMethod === "clipboard.readText" || "the palette field never reached clipboard.readText",
      () => Check.find(fieldLoader.item, "paletteField").text === root.pastedInto
        || ("the palette field's text is wrong: " + Check.find(fieldLoader.item, "paletteField").text),

      // Ctrl+Shift+Insert is one of the keys ui/lib/Paste.js's own
      // isPasteKey recognises, not matched by field.paste's own two
      // listed keys; the palette field must still catch it itself.
      () => { fakeService.lastMethod = ""; Check.find(fieldLoader.item, "paletteField").text = ""; return true; },
      () => { t.keyClick(Qt.Key_Insert, Qt.ControlModifier | Qt.ShiftModifier); return true; },
      () => fakeService.lastMethod === "clipboard.readText" || "Ctrl+Shift+Insert in the palette field never reached clipboard.readText",
      () => Check.find(fieldLoader.item, "paletteField").text === root.pastedInto
        || ("Ctrl+Shift+Insert left the palette field's text wrong: " + Check.find(fieldLoader.item, "paletteField").text),

      () => { fakeService.lastMethod = ""; fieldLoader.sourceComponent = searchComponent; return true; },
      () => !!Check.find(fieldLoader.item, "searchField")?.activeFocus,
      () => { t.keyClick(Qt.Key_Insert, Qt.ShiftModifier); return true; },
      () => fakeService.lastMethod === "clipboard.readText" || "the search field never reached clipboard.readText",
      () => Check.find(fieldLoader.item, "searchField").text === root.pastedInto
        || ("the search field's text is wrong: " + Check.find(fieldLoader.item, "searchField").text),

      // The dedicated Paste key some keyboards and platform themes
      // send (Qt.Key_Paste) carries no modifier at all; isPasteKey
      // recognises it directly, rather than through Ctrl+V or
      // Shift+Insert.
      () => { fakeService.lastMethod = ""; Check.find(fieldLoader.item, "searchField").text = ""; return true; },
      () => { t.keyClick(Qt.Key_Paste); return true; },
      () => fakeService.lastMethod === "clipboard.readText" || "Key_Paste in the search field never reached clipboard.readText",
      () => Check.find(fieldLoader.item, "searchField").text === root.pastedInto
        || ("Key_Paste left the search field's text wrong: " + Check.find(fieldLoader.item, "searchField").text),

      () => { fakeService.lastMethod = ""; fieldLoader.sourceComponent = setupComponent; return true; },
      () => !!Check.find(fieldLoader.item, "apiIdField")?.activeFocus,
      () => { t.keyClick(Qt.Key_V, Qt.ControlModifier); return true; },
      () => fakeService.lastMethod === "clipboard.readText" || "the setup field never reached clipboard.readText",
      () => Check.find(fieldLoader.item, "apiIdField").text === root.pastedInto
        || ("the setup field's text is wrong: " + Check.find(fieldLoader.item, "apiIdField").text),

      // With field.paste rebound off Ctrl+V entirely (as keys.conf
      // could do), Ctrl+V must still paste: the field's own isPasteKey
      // interception runs ahead of, and regardless of, that binding
      // lookup.
      () => { fakeService.lastMethod = ""; Check.find(fieldLoader.item, "apiIdField").text = ""; root.bindings = root.reboundBindings; return true; },
      () => { t.keyClick(Qt.Key_V, Qt.ControlModifier); return true; },
      () => fakeService.lastMethod === "clipboard.readText" || "Ctrl+V with field.paste rebound never reached clipboard.readText",
      () => Check.find(fieldLoader.item, "apiIdField").text === root.pastedInto
        || ("Ctrl+V with field.paste rebound left the setup field's text wrong: " + Check.find(fieldLoader.item, "apiIdField").text)
    ]
  }
}
