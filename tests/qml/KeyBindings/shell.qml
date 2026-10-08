// Checks that ~/.config/omamessenger/keys.conf's overrides really change
// key routing, not just the pure merge logic tests/unit/keybindings.test.cjs
// already covers: the fixture at config/omamessenger/keys.conf (copied into
// this test's own XDG_CONFIG_HOME by the test runner, so it is already on
// disk before Quickshell even starts) remaps the command palette's keys
// onto Ctrl+Alt+P and reports two errors (an unknown action and a
// malformed key). The test drives the real Panel and Service: the old
// default (Ctrl+/) must do nothing, the new key must open the palette,
// the footer's key hints must show the new key, the palette's own
// "Show key bindings" command must list the override, and editing the
// file at runtime must take effect without restarting anything.
import QtQuick
import QtTest
import Quickshell
import Quickshell.Io
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property int pollAttempts: 0

  // retry schedules fn to run again shortly, for a condition that depends
  // on the helper's async file load or a key event finishing its round trip.
  function retry(fn: var): void {
    root._next = fn;
    retryTimer.start();
  }
  property var _next: null

  Service {
    id: service
  }

  QtObject {
    id: fakeShell

    function hide(id) { panel.close(); }
    function serviceFor(id) { return service; }
    function toggle(id, payloadJson) { panel.open(payloadJson); }
    function summon(id, payloadJson) { panel.open(payloadJson); }
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

  Timer {
    id: retryTimer
    interval: 100
    onTriggered: root._next()
  }

  // A deliberately unreachable deadline: it only fires, and fails the
  // test with a reason, if something above never happens.
  Timer {
    running: true
    interval: 20000
    onTriggered: Check.fail("timed out before the checks finished")
  }

  // Checks run once Quickshell has finished loading; Qt.exit() is
  // ignored before then.
  Timer {
    running: true
    interval: 50
    onTriggered: root.start()
  }

  // start opens the window, which is enough to start the service's own
  // keys.conf read; the rest waits for that read to finish.
  function start(): void {
    panel.open("{}");
    root.waitForOverrideLoaded();
  }

  // waitForOverrideLoaded holds until Service has merged the fixture file,
  // visible as palette.commands' effective key changing off its default.
  function waitForOverrideLoaded(): void {
    const binding = service.effectiveBindings.find((b) => b.action === "palette.commands");

    if (binding && binding.keys[0] === "Ctrl+Alt+P") return root.checkErrorsReported();

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("palette.commands never picked up the fixture's override; keys are "
        + JSON.stringify(binding ? binding.keys : null));
    root.retry(root.waitForOverrideLoaded);
  }

  // checkErrorsReported checks the fixture's deliberate mistakes were
  // reported and nothing else was mistaken for a conflict: the unknown
  // action is one error, and the malformed key is two (the bad token
  // itself, then the action being left with no valid keys at all).
  function checkErrorsReported(): void {
    if (service.keyBindingErrors.length !== 3)
      return Check.fail("keyBindingErrors has " + service.keyBindingErrors.length + " entries, want 3: "
        + JSON.stringify(service.keyBindingErrors));
    if (service.keyBindingConflicts.length !== 0)
      return Check.fail("keyBindingConflicts is not empty: " + JSON.stringify(service.keyBindingConflicts));

    root.checkOldDefaultInert();
  }

  // checkOldDefaultInert presses the palette's old default (Ctrl+/) and
  // checks it does nothing now that the override replaced it.
  function checkOldDefaultInert(): void {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);

    const palette = Check.find(panel, "commandPalette");
    if (palette && palette.visible) return Check.fail("Ctrl+/ still opened the palette after overriding palette.commands");

    root.checkNewKeyOpensPalette();
  }

  // checkNewKeyOpensPalette presses the fixture's override key for
  // opening the palette itself, then checks a second overridden action
  // (chat.new) shows its own new key in the palette's listing, not its
  // default: the command palette is the first of the brief's three
  // surfaces ("the command palette, footer hints, key-hint rows") that
  // must reflect the effective keys, not just Keymap's defaults.
  function checkNewKeyOpensPalette(): void {
    t.keyClick(Qt.Key_P, Qt.ControlModifier | Qt.AltModifier);

    const palette = Check.find(panel, "commandPalette");
    if (!palette || !palette.visible) return Check.fail("Ctrl+Alt+P did not open the command palette");

    for (const ch of "new mes") t.keyClick(ch);
    if (palette.items.length === 0 || palette.items[0].label !== "New message")
      return Check.fail("typing \"new mes\" listed " + JSON.stringify(palette.items.map((i) => i.label)));
    if (palette.items[0].keys !== "Ctrl+Alt+N")
      return Check.fail("New message's palette row shows \"" + palette.items[0].keys + "\", want \"Ctrl+Alt+N\"");

    t.keyClick(Qt.Key_Escape);
    root.checkFooterShowsNewKey();
  }

  // checkFooterShowsNewKey checks the footer's key hints (the brief's
  // "footer hints" and "key-hint rows" surfaces) switched to both
  // overridden actions' new keys, and dropped their old defaults.
  function checkFooterShowsNewKey(): void {
    const hints = Check.find(panel, "keyHints");
    if (!hints) return Check.fail("could not find the footer's key hints");

    if (hints.text.indexOf("Ctrl+Alt+P") === -1 || hints.text.indexOf("Ctrl+Alt+N") === -1)
      return Check.fail("footer hints do not show the overridden keys: " + JSON.stringify(hints.text));
    if (hints.text.indexOf("Ctrl+/") !== -1 || hints.text.indexOf("Ctrl+N ") !== -1)
      return Check.fail("footer hints still show an old default: " + JSON.stringify(hints.text));

    root.checkShowKeyBindingsCommand();
  }

  // checkShowKeyBindingsCommand runs "Show key bindings" from the palette
  // and checks the listing names the override and both reported errors.
  function checkShowKeyBindingsCommand(): void {
    t.keyClick(Qt.Key_P, Qt.ControlModifier | Qt.AltModifier);
    for (const ch of "show key bind") t.keyClick(ch);

    const palette = Check.find(panel, "commandPalette");
    if (!palette || palette.items.length === 0 || palette.items[0].label !== "Show key bindings")
      return Check.fail("typing \"show key bind\" listed " + JSON.stringify(palette ? palette.items.map((i) => i.label) : []));

    t.keyClick(Qt.Key_Return);
    const labels = palette.items.map((i) => i.label);
    if (!labels.some((l) => l.indexOf("Error (line") === 0))
      return Check.fail("Show key bindings did not list the fixture's errors: " + JSON.stringify(labels));
    if (!labels.includes("Command palette"))
      return Check.fail("Show key bindings did not list Command palette: " + JSON.stringify(labels));

    const commandRow = palette.items.find((i) => i.label === "Command palette");
    if (commandRow.detail !== "custom" || commandRow.keys !== "Ctrl+Alt+P")
      return Check.fail("Show key bindings did not mark Command palette as custom with its new key: "
        + JSON.stringify(commandRow));

    t.keyClick(Qt.Key_Escape);
    root.rewriteConfigFile();
  }

  // rewriteConfigFile overwrites the fixture with a different key for the
  // same action, the way editing it by hand would, to check the live
  // reload path rather than only the startup load already checked above.
  function rewriteConfigFile(): void {
    rewriter.setText("palette.commands = Ctrl+Alt+O\n");
    root.pollAttempts = 0;
    root.waitForReload();
  }

  FileView {
    id: rewriter
    path: Quickshell.env("XDG_CONFIG_HOME") + "/omamessenger/keys.conf"
    printErrors: false
  }

  // waitForReload holds until Service's own watch on the same file picks
  // up the rewrite and re-merges it.
  function waitForReload(): void {
    const binding = service.effectiveBindings.find((b) => b.action === "palette.commands");

    if (binding && binding.keys[0] === "Ctrl+Alt+O") return root.checkReloadedKeyWorks();

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("editing keys.conf was never picked up; palette.commands keys are still "
        + JSON.stringify(binding ? binding.keys : null));
    root.retry(root.waitForReload);
  }

  // checkReloadedKeyWorks checks the previous override key stopped
  // working and the file's new key opens the palette instead.
  function checkReloadedKeyWorks(): void {
    t.keyClick(Qt.Key_P, Qt.ControlModifier | Qt.AltModifier);
    if (Check.find(panel, "commandPalette").visible)
      return Check.fail("the old override key (Ctrl+Alt+P) still opened the palette after the file changed");

    t.keyClick(Qt.Key_O, Qt.ControlModifier | Qt.AltModifier);
    if (!Check.find(panel, "commandPalette").visible)
      return Check.fail("the file's new key (Ctrl+Alt+O) did not open the palette after reloading");

    console.log("PASS KeyBindings");
    Qt.exit(0);
  }
}
