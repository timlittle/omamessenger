// Checks NewChatContactRow's highlight: at rest it is fully transparent,
// and hovering it shows the quiet hover fill. An audit found this row had
// the selected/not-selected branches every other row has, but no hover
// fallback, so a contact never lit up under the pointer; this is the
// regression test for that gap. Also checks that a contact's name
// renders exactly as the helper gives it, including a bare,
// self-chosen name with no saved contact behind it.
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
    implicitHeight: 160
    visible: true

    Column {
      id: container
      anchors.fill: parent
      spacing: 0

      NewChatContactRow {
        id: contactRow
        width: 280
        contact: ({ accountId: "a1", remoteId: "r1", name: "Ben Okafor" })
        current: false
      }

      NewChatContactRow {
        id: selfChosenRow
        width: 280
        contact: ({ accountId: "a1", remoteId: "r2", name: "Mum" })
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

  // run moves the pointer on and off the row, checks the fill colour,
  // then checks that both a saved contact's name and a bare,
  // self-chosen name render exactly as given.
  function run(): void {
    t.mouseMove(container, 2, container.height - 2); // start away from the row
    t.wait(50);
    if (contactRow.color.a !== 0)
      return Check.fail("an unhovered, unselected row is not transparent: " + contactRow.color);

    t.mouseMove(contactRow, contactRow.width / 2, contactRow.height / 2);
    t.wait(50);
    if (contactRow.color.a === 0)
      return Check.fail("hovering the row shows no hover fill: still " + contactRow.color);

    if (!checkNamesRenderPlainly())
      return;

    console.log("PASS NewChatContactRow");
    Qt.exit(0);
  }

  // checkNamesRenderPlainly confirms both a saved contact's name and a
  // bare, self-chosen name show exactly as given, with no marker.
  function checkNamesRenderPlainly(): bool {
    const name = Check.find(contactRow, "contactName");
    if (!name || name.text !== "Ben Okafor")
      return Check.fail("contact name = " + (name && name.text) + ", want Ben Okafor");

    const selfChosenName = Check.find(selfChosenRow, "contactName");
    if (!selfChosenName || selfChosenName.text !== "Mum")
      return Check.fail("self-chosen contact name = " + (selfChosenName && selfChosenName.text) + ", want Mum");

    return true;
  }
}
