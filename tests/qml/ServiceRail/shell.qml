// Checks the service rail: it has one entry per Rail.items() result,
// clicking an entry emits selected() with its key, a busy rail (long
// account names, four-digit unread counts, every status dot) never
// squashes an entry's unread badge into its glyph, and an account entry
// (but not a service entry covering just one account) shows its own
// account colour dot.
import QtQuick
import QtTest
import Quickshell
import qs.Commons
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
  property var accountColors: ({ a1: "#ff00ff00", a2: "#ff0000ff" })

  // busyAccounts: several Telegram accounts with long names and a service
  // other than Telegram, to reproduce the rail "busy" with many entries.
  property var busyAccounts: [
    { id: "b1", service: "telegram", name: "Telegram Operations And Escalations Desk", status: "error" },
    { id: "b2", service: "telegram", name: "Telegram Customer Support Backup Line", status: "needs-auth" },
    { id: "b3", service: "telegram", name: "Telegram Personal Family Archive Chat", status: "connecting" },
    { id: "b4", service: "whatsapp", name: "Work", status: "connected" }
  ]
  // busyConversations: unread counts chosen to require 1, 2 and 4-digit
  // badge text ("7", "96", "99+" for 1234), the case that squashed the glyph.
  property var busyConversations: [
    { id: "c1", service: "telegram", accountId: "b1", unread: 7 },
    { id: "c2", service: "telegram", accountId: "b2", unread: 96 },
    { id: "c3", service: "telegram", accountId: "b3", unread: 1234 },
    { id: "c4", service: "whatsapp", accountId: "b4", unread: 3 }
  ]
  property var busyItems: Rail.items(root.busyAccounts, root.busyConversations)

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
      accountColors: root.accountColors
      onSelected: key => root.selectedKeys.push(key)
    }
  }

  // busyWin: the rail at its real fixed width, the width the panel always
  // gives it, with the busy entry set that squashed badge into glyph.
  FloatingWindow {
    id: busyWin
    implicitWidth: Style.space(64)
    implicitHeight: 480
    visible: true

    ServiceRail {
      id: busyRail
      anchors.fill: parent
      items: root.busyItems
      selectedKey: "all"
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

    for (const item of root.busyItems) {
      if (!root.checkBusyEntry(item.key)) return;
    }

    const accountEntry = Check.find(rail, "entry-account:a1");
    if (!accountEntry) return Check.fail("entry-account:a1 not found");
    const accountDot = Check.find(accountEntry, "accountColorDot");
    if (!accountDot || !accountDot.visible)
      return Check.fail("account entry a1 does not show its account colour dot");
    if (String(accountDot.color) !== "#00ff00")
      return Check.fail("account entry a1's dot is not tinted with its account colour: " + accountDot.color);

    const serviceEntry = Check.find(rail, "entry-service:whatsapp");
    const serviceDot = Check.find(serviceEntry, "accountColorDot");
    if (!serviceDot || serviceDot.visible)
      return Check.fail("a service entry covering one account still shows an account colour dot");

    console.log("PASS ServiceRail");
    Qt.exit(0);
  }

  // checkBusyEntry finds one busy-rail entry by its key and verifies its
  // badge never overlaps its glyph, and that the badge and label stay
  // inside the entry, however long the name or however large the count.
  // It returns false after Check.fail so run() can stop at the first
  // broken entry.
  function checkBusyEntry(key): bool {
    const entryItem = Check.find(busyRail, "entry-" + key);
    if (!entryItem) return Check.fail("entry-" + key + " not found");

    const glyph = Check.find(entryItem, "glyph");
    const badge = Check.find(entryItem, "badge");
    const label = Check.find(entryItem, "label");
    if (!glyph || !badge || !label) return Check.fail(key + ": missing glyph, badge or label");

    const glyphRect = Check.rect(glyph, entryItem);
    const badgeRect = Check.rect(badge, entryItem);
    const labelRect = Check.rect(label, entryItem);

    // "A fixed corner of the glyph" means level with it, not a row of its
    // own further down the entry: the two must share some of the same
    // vertical space, even though (checked next) they must never overlap.
    const sharedRow = Math.min(glyphRect.y + glyphRect.height, badgeRect.y + badgeRect.height)
      - Math.max(glyphRect.y, badgeRect.y);
    if (sharedRow <= 0)
      return Check.fail(key + ": badge " + JSON.stringify(badgeRect) + " is not level with glyph "
        + JSON.stringify(glyphRect) + " (stacked in its own row instead of at a corner)");

    // A one-pixel tolerance absorbs font-metric rounding (glyph widths such
    // as 17.390625 above) without hiding a real overlap.
    const overlap = Check.overlapArea(glyphRect, badgeRect, 1);
    if (overlap > 0)
      return Check.fail(key + ": badge overlaps glyph by " + overlap + " px^2 (glyph "
        + JSON.stringify(glyphRect) + ", badge " + JSON.stringify(badgeRect) + ")");

    if (badgeRect.x < -1 || badgeRect.x + badgeRect.width > entryItem.width + 1)
      return Check.fail(key + ": badge " + JSON.stringify(badgeRect) + " spills outside the "
        + entryItem.width + "px entry");

    if (labelRect.x < -1 || labelRect.x + labelRect.width > entryItem.width + 1)
      return Check.fail(key + ": label " + JSON.stringify(labelRect) + " spills outside the "
        + entryItem.width + "px entry");

    return true;
  }
}
