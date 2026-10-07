// Checks Ctrl+K's own ordering and its unread badge: WindowController's
// paletteItems list every unread conversation before the read ones, most
// recently active first within each group, typing a query still narrows to
// matches while two equally-good matches keep that same unread-first order,
// and each row carries its own unread count for PaletteRow to show as a
// badge, which PaletteRow renders visibly for an unread chat and hides for
// a read one, never relying on the badge's accent colour alone since the
// count text itself names it.
import QtQuick
import Quickshell
import "ui/components"
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  // service answers conversations.list with a mix of unread and read
  // chats across two services, deliberately not in the order the test
  // expects them read back, so the check exercises WindowController's own
  // sort rather than one the fixture already did for it.
  QtObject {
    id: service

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", query: "", drafts: {} })
    property var services: []

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") callback(null, [
        { id: "r1", title: "Read Older", service: "whatsapp", lastActivity: 100, unread: 0 },
        { id: "u1", title: "Unread Older", service: "whatsapp", lastActivity: 200, unread: 3 },
        { id: "r2", title: "Read Newer", service: "telegram", lastActivity: 400, unread: 0 },
        { id: "u2", title: "Unread Newer", service: "telegram", lastActivity: 300, unread: 1 },
        { id: "z1", title: "Zorro Read", service: "whatsapp", lastActivity: 10, unread: 0 },
        { id: "z2", title: "Zorro Unread", service: "whatsapp", lastActivity: 5, unread: 2 }
      ]);
    }
  }

  ListController {
    id: listController
    service: service
  }

  WindowController {
    id: windowController
    service: service
    listController: listController
  }

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 400
    visible: true

    Column {
      anchors.fill: parent

      PaletteRow {
        id: unreadRow
        width: 300
        item: ({ label: "Unread chat", detail: "", keys: "", unread: 4 })
      }

      PaletteRow {
        id: readRow
        width: 300
        item: ({ label: "Read chat", detail: "", keys: "", unread: 0 })
      }
    }
  }

  Timer {
    running: true
    interval: 50
    onTriggered: root.run()
  }

  // run checks the unfiltered order, then a query narrowing to an
  // equally-scored tie, then the badge each row carries.
  function run(): void {
    windowController.run("palette.conversations");

    const order = windowController.paletteItems.map((i) => i.label);
    if (JSON.stringify(order) !== JSON.stringify(["Unread Newer", "Unread Older", "Zorro Unread", "Read Newer", "Read Older", "Zorro Read"]))
      return Check.fail("Ctrl+K did not list unread chats first, most recent within each group: " + JSON.stringify(order));

    windowController.setPaletteQuery("zorro");
    const matches = windowController.paletteResults.map((c) => c.id);
    if (JSON.stringify(matches) !== '["z2","z1"]')
      return Check.fail("a query did not keep the unread-first order among equally good matches: " + JSON.stringify(matches));

    root.checkBadge();
  }

  // checkBadge confirms PaletteRow shows the count for an unread row and
  // hides the badge entirely for a read one.
  function checkBadge(): void {
    const shown = Check.find(unreadRow, "unreadBadge");
    if (!shown || !shown.visible)
      return Check.fail("PaletteRow does not show the unread badge for a chat with unread messages");

    const hidden = Check.find(readRow, "unreadBadge");
    if (!hidden || hidden.visible)
      return Check.fail("PaletteRow shows an unread badge for a read chat");

    console.log("PASS Palette");
    Qt.exit(0);
  }
}
