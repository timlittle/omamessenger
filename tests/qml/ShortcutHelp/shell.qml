// Checks ShortcutHelp: the rendered section titles match Keymap.helpSections().
import QtQuick
import Quickshell
import "ui/components"
import "ui/lib/Keymap.js" as Keymap

ShellRoot {
  id: root

  // fail stops the test with a reason on stderr and reports failure to
  // its caller, so run() can bail out with "return root.fail(...)".
  function fail(reason: string): bool {
    console.error("FAIL " + reason);
    Qt.exit(1);
    return false;
  }

  // collect walks the item tree gathering every node with a text property.
  function collect(item, out) {
    if (typeof item.text === "string") out.push(item);
    for (const child of item.children) collect(child, out);
  }

  FloatingWindow {
    implicitWidth: 700
    implicitHeight: 500
    visible: true

    ShortcutHelp {
      id: help
      anchors.fill: parent
      open: true
    }
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run checks every help section title is rendered, stopping at the
  // first missing one so Qt.exit(1) is the one that takes effect.
  function run(): void {
    const nodes = [];
    root.collect(help, nodes);
    const titles = nodes.map(node => node.text);

    for (const section of Keymap.helpSections()) {
      if (!titles.includes(section.title)) {
        root.fail("section title \"" + section.title + "\" not rendered");
        return;
      }
    }

    console.log("PASS ShortcutHelp");
    Qt.exit(0);
  }
}
