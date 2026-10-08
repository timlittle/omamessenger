// Checks NewChatContactRow's highlight: at rest it is fully transparent,
// and hovering it shows the quiet hover fill. An audit found this row had
// the selected/not-selected branches every other row has, but no hover
// fallback, so a contact never lit up under the pointer; this is the
// regression test for that gap.
import QtQuick
import QtTest
import Quickshell
import qs.Commons
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  FloatingWindow {
    id: win
    implicitWidth: 300
    implicitHeight: 100
    visible: true

    Item {
      id: container
      anchors.fill: parent

      NewChatContactRow {
        id: contactRow
        width: 280
        contact: ({ accountId: "a1", remoteId: "r1", name: "Ben Okafor" })
        current: false
      }
    }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    running: true
    interval: 50
    onTriggered: root.run()
  }

  // run moves the pointer on and off the row and checks the fill colour.
  function run(): void {
    t.mouseMove(container, 2, container.height - 2); // start away from the row
    t.wait(50);
    if (contactRow.color.a !== 0)
      return Check.fail("an unhovered, unselected row is not transparent: " + contactRow.color);

    t.mouseMove(contactRow, contactRow.width / 2, contactRow.height / 2);
    t.wait(50);
    if (contactRow.color.a === 0)
      return Check.fail("hovering the row shows no hover fill: still " + contactRow.color);

    console.log("PASS NewChatContactRow");
    Qt.exit(0);
  }
}
