// Checks KeyHints: it joins each hinted binding's primary key and label
// for the active context, in the order Keymap.bindingsFor returns them.
// Which bindings are hinted, and in what order, is Keymap.js's own job,
// already proven in tests/unit/keymap.test.cjs; this only checks the
// component shows the text that function builds - in particular, that
// Ctrl+L Send later shows up in the compose context (the only visible
// sign that shortcut exists besides the clock button's own tooltip).
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  FloatingWindow {
    id: win
    implicitWidth: 500
    implicitHeight: 100
    visible: true

    KeyHints {
      id: hints
      context: "compose"
    }
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  function run(): void {
    if (!root.checkShowsSendLater()) return;
    if (!root.checkSwitchesWithContext()) return;

    console.log("PASS KeyHints");
    Qt.exit(0);
  }

  function checkShowsSendLater(): bool {
    if (!hints.text.includes("Ctrl+L Send later")) {
      return Check.fail("compose context did not hint Send later: \"" + hints.text + "\"");
    }
    return true;
  }

  function checkSwitchesWithContext(): bool {
    hints.context = "list";
    if (hints.text.includes("Send later")) {
      return Check.fail("the list context should not hint a compose-only action: \"" + hints.text + "\"");
    }
    if (!hints.text.includes("Open chat")) {
      return Check.fail("the list context did not hint opening a chat: \"" + hints.text + "\"");
    }
    return true;
  }
}
