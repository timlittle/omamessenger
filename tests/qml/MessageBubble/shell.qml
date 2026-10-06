// Checks that message bubbles fit their text: a short message gets a
// small bubble, a long one stops at 72% of the width and wraps.
import QtQuick
import Quickshell
import "ui/components"

ShellRoot {
  id: root

  readonly property var noAnnotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })

  // fail stops the test with a reason on stderr.
  function fail(reason: string): void {
    console.error("FAIL " + reason);
    Qt.exit(1);
  }

  // find returns the descendant of item with the given objectName.
  function find(item: Item, name: string): Item {
    if (item.objectName === name)
      return item;
    for (let i = 0; i < item.children.length; i++) {
      const found = root.find(item.children[i], name);
      if (found)
        return found;
    }
    return null;
  }

  // run measures both bubbles once they have been laid out.
  function run(): void {
    const short = root.find(shortMessage, "bubble");
    const long = root.find(longMessage, "bubble");
    const max = 600 * 0.72;

    if (short.width > 600 * 0.3)
      return fail("short bubble is " + short.width + "px wide, want a small bubble");
    if (Math.abs(long.width - max) > 1)
      return fail("long bubble is " + long.width + "px wide, want " + max);
    const lines = root.find(longMessage, "body").lineCount;
    if (lines < 2)
      return fail("long message shows on " + lines + " line, want it wrapped");

    console.log("PASS MessageBubble");
    Qt.exit(0);
  }

  FloatingWindow {
    implicitWidth: 600
    implicitHeight: 400
    visible: true

    Column {
      width: 600

      MessageDelegate {
        id: shortMessage

        width: 600
        annotation: root.noAnnotation
        message: ({ id: "a", text: "hi", outgoing: true, status: "read", created: 0, senderName: "" })
      }

      MessageDelegate {
        id: longMessage

        width: 600
        annotation: root.noAnnotation
        message: ({
          id: "b", outgoing: false, status: "received", created: 0, senderName: "",
          text: "This is a long message that goes on and on and should wrap across several lines in its bubble"
        })
      }
    }
  }

  // Layout settles over a few frames, so measure after a short delay.
  Timer {
    running: true
    interval: 100
    onTriggered: root.run()
  }
}
