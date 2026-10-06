// Checks the layout in a window too narrow for the list and a conversation
// side by side, such as a half-screen tile: the list fills the space,
// opening a conversation swaps it for the conversation, and closing it
// brings the list back.
import QtQuick
import Quickshell
import "ui"
import "ui/components"
import "ui/controllers"

ShellRoot {
  id: root

  property int step: 0
  property int attempts: 0
  property var steps: [root.waitForList, root.openFirst, root.checkConversationOnly, root.closeIt, root.checkListOnly]

  // fail stops the test with a reason on stderr.
  function fail(reason: string): void {
    console.error("FAIL step " + root.step + ": " + reason);
    Qt.exit(1);
  }

  // find returns the descendant of item with the given objectName.
  function find(item: var, name: string): var {
    if (!item) return null;
    if (item.objectName === name) return item;

    const kids = item.children;
    for (let i = 0; i < kids.length; i++) {
      const found = root.find(kids[i], name);
      if (found) return found;
    }
    return null;
  }

  // waitForList holds until the demo conversations fill a full-width list
  // and no conversation column is shown.
  function waitForList(): var {
    if (listController.model.count !== 11) return false;
    return root.checkListOnly();
  }

  // checkListOnly checks the list is the only column and fills the space.
  function checkListOnly(): var {
    const listColumn = root.find(layout, "listColumn");
    const conversation = root.find(layout, "conversationView");
    if (!layout.narrow) return "a 600-wide window is not treated as narrow";
    if (!listColumn.visible || conversation.visible) return "narrow window shows the conversation instead of the list";
    if (listColumn.width < 400) return "the list is only " + listColumn.width + " wide";
    return true;
  }

  // openFirst opens the first conversation.
  function openFirst(): var {
    conversationController.open(listController.model.get(0));
    return true;
  }

  // checkConversationOnly checks the conversation replaced the list.
  function checkConversationOnly(): var {
    const listColumn = root.find(layout, "listColumn");
    const conversation = root.find(layout, "conversationView");
    if (listColumn.visible) return "the list is still showing beside the open conversation";
    if (!conversation.visible || conversation.width < 400) return "the conversation is not filling the window";
    return true;
  }

  // closeIt closes the conversation again.
  function closeIt(): var {
    conversationController.close();
    return true;
  }

  // runStep runs the current step and advances, retries or fails.
  function runStep(): void {
    const result = root.steps[root.step]();
    if (typeof result === "string" && root.step > 0) return root.fail(result);

    if (result === true) {
      root.step++;
      root.attempts = 0;
      if (root.step === root.steps.length) {
        console.log("PASS NarrowLayout");
        return Qt.exit(0);
      }
    } else if (++root.attempts > 100) {
      return root.fail(typeof result === "string" ? result : "condition not met within 10 s");
    }

    stepTimer.start();
  }

  Service {
    id: helperService
  }

  ListController {
    id: listController

    service: helperService
  }

  ConversationController {
    id: conversationController

    service: helperService
    listController: listController
  }

  DialogController {
    id: dialogController

    service: helperService
  }

  WindowController {
    id: windowController

    service: helperService
    listController: listController
    conversationController: conversationController
    dialogController: dialogController
  }

  FloatingWindow {
    implicitWidth: 600
    implicitHeight: 700
    visible: true

    MessengerLayout {
      id: layout

      anchors.fill: parent
      service: helperService
      listController: listController
      conversationController: conversationController
      dialogController: dialogController
      windowController: windowController
    }
  }

  Timer {
    id: stepTimer

    interval: 100
    onTriggered: root.runStep()
  }

  // Start once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 50
    onTriggered: root.runStep()
  }
}
