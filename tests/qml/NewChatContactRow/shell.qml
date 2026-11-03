// Checks NewChatContactRow's highlight: at rest it is fully transparent,
// and hovering it shows the quiet hover fill. An audit found this row had
// the selected/not-selected branches every other row has, but no hover
// fallback, so a contact never lit up under the pointer; this is the
// regression test for that gap. Also checks that an unverified contact's
// name is marked, since the helper's own contact list can carry a bare,
// self-chosen name with no saved contact behind it (see
// docs/decisions.md).
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
        contact: ({ accountId: "a1", remoteId: "r1", name: "Ben Okafor", verified: true })
        current: false
      }

      NewChatContactRow {
        id: unverifiedRow
        width: 280
        contact: ({ accountId: "a1", remoteId: "r2", name: "Mum", verified: false })
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
  // then checks that a verified contact's name is unmarked and an
  // unverified one is marked with "~".
  function run(): void {
    t.mouseMove(container, 2, container.height - 2); // start away from the row
    t.wait(50);
    if (contactRow.color.a !== 0)
      return Check.fail("an unhovered, unselected row is not transparent: " + contactRow.color);

    t.mouseMove(contactRow, contactRow.width / 2, contactRow.height / 2);
    t.wait(50);
    if (contactRow.color.a === 0)
      return Check.fail("hovering the row shows no hover fill: still " + contactRow.color);

    if (!checkVerifiedStatusIsMarked())
      return;

    console.log("PASS NewChatContactRow");
    Qt.exit(0);
  }

  // checkVerifiedStatusIsMarked confirms a verified contact's name shows
  // unmarked and an unverified one shows with a leading "~".
  function checkVerifiedStatusIsMarked(): bool {
    const verifiedName = Check.find(contactRow, "contactName");
    if (!verifiedName || verifiedName.text !== "Ben Okafor")
      return Check.fail("verified contact name = " + (verifiedName && verifiedName.text) + ", want unmarked Ben Okafor");

    const unverifiedName = Check.find(unverifiedRow, "contactName");
    if (!unverifiedName || unverifiedName.text !== "~Mum")
      return Check.fail("unverified contact name = " + (unverifiedName && unverifiedName.text) + ", want ~Mum");

    return true;
  }
}
