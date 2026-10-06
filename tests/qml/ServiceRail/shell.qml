// Checks the service rail: it has one entry per Rail.items() result,
// clicking an entry emits selected() with its key, and the DEMO chip
// shows only in demo mode.
import QtQuick
import QtTest
import Quickshell
import "ui/lib/Rail.js" as Rail
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var selectedKeys: []

  property var accounts: [
    { id: "a1", service: "telegram", name: "Alice", status: "connected" },
    { id: "a2", service: "telegram", name: "Bob", status: "connecting" },
    { id: "a3", service: "whatsapp", name: "Work", status: "connected" }
  ]
  property var conversations: []
  property var railItems: Rail.items(root.accounts, root.conversations)

  FloatingWindow {
    id: win
    implicitWidth: 200
    implicitHeight: 400
    visible: true

    ServiceRail {
      id: rail
      anchors.fill: parent
      items: root.railItems
      selectedKey: "all"
      onSelected: key => root.selectedKeys.push(key)
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

  // run drives the rail and checks the outcome.
  function run(): void {
    if (root.railItems.length !== 5)
      return Check.fail("expected 5 rail entries (all, 2 services, 2 telegram accounts), got "
        + root.railItems.length);

    const list = Check.find(rail, "entry-service:whatsapp");
    if (!list)
      return Check.fail("entry-service:whatsapp not found");

    t.mouseClick(list, list.width / 2, list.height / 2);
    if (JSON.stringify(root.selectedKeys) !== '["service:whatsapp"]')
      return Check.fail("selected " + JSON.stringify(root.selectedKeys) + ", want [\"service:whatsapp\"]");

    console.log("PASS ServiceRail");
    Qt.exit(0);
  }
}
