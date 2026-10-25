// Checks ScheduledList: each queued message shows its own send time and
// text, the list takes no space with nothing queued, and each row's
// send-now, edit and cancel buttons report the right message id. What
// counts as the send-time label is Format.sendAtLabel's own job,
// already proven in tests/unit/format.test.cjs; this only checks the
// component shows it and wires its buttons.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var sentNowIds: []
  property var editedRows: []
  property var cancelledIds: []

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 200
    visible: true

    ScheduledList {
      id: list
      width: 360
      // A fixed clock, so the send-time label is exactly reproducible:
      // 2026-01-01T10:00 local time.
      nowMs: new Date(2026, 0, 1, 10, 0).getTime()
      messages: [
        { id: "s1", text: "see you soon", sendAt: new Date(2026, 0, 1, 18, 0).getTime() },
        { id: "s2", text: "happy birthday", sendAt: new Date(2026, 0, 2, 9, 0).getTime() }
      ]
      onSendNowRequested: id => root.sentNowIds.push(id)
      onEditRequested: (id, text) => root.editedRows.push({ id: id, text: text })
      onCancelRequested: id => root.cancelledIds.push(id)
    }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  function run(): void {
    if (!root.checkRowsShowTimeAndText()) return;
    if (!root.checkHiddenWithNothingQueued()) return;
    if (!root.checkButtonsReportTheRightMessage()) return;

    console.log("PASS ScheduledList");
    Qt.exit(0);
  }

  function checkRowsShowTimeAndText(): bool {
    const shown = Check.texts(list).map((item) => item.text);
    if (!shown.includes("Sends at 18:00")) return Check.fail("first row's send time not shown: " + JSON.stringify(shown));
    if (!shown.includes("see you soon")) return Check.fail("first row's text not shown: " + JSON.stringify(shown));
    if (!shown.includes("Sends at Tomorrow 09:00")) return Check.fail("second row's send time not shown: " + JSON.stringify(shown));
    if (!shown.includes("happy birthday")) return Check.fail("second row's text not shown: " + JSON.stringify(shown));
    return true;
  }

  function checkHiddenWithNothingQueued(): bool {
    list.messages = [];
    if (list.visible) return Check.fail("ScheduledList stayed visible with nothing queued");
    list.messages = [{ id: "s1", text: "see you soon", sendAt: new Date(2026, 0, 1, 18, 0).getTime() }];
    if (!list.visible) return Check.fail("ScheduledList did not become visible again once something was queued");
    return true;
  }

  // checkButtonsReportTheRightMessage drives the one row's own buttons
  // (messages is down to one row by now, so Check.find's first match is
  // unambiguous) and confirms each reports s1's own id.
  function checkButtonsReportTheRightMessage(): bool {
    t.mouseClick(Check.find(list, "scheduledSendNowButton"));
    if (JSON.stringify(root.sentNowIds) !== '["s1"]') return Check.fail("send-now button reported " + JSON.stringify(root.sentNowIds));

    t.mouseClick(Check.find(list, "scheduledEditButton"));
    if (root.editedRows.length !== 1 || root.editedRows[0].id !== "s1" || root.editedRows[0].text !== "see you soon")
      return Check.fail("edit button reported " + JSON.stringify(root.editedRows));

    t.mouseClick(Check.find(list, "scheduledCancelButton"));
    if (JSON.stringify(root.cancelledIds) !== '["s1"]') return Check.fail("cancel button reported " + JSON.stringify(root.cancelledIds));

    return true;
  }
}
