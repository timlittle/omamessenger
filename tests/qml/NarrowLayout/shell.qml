// Checks the layout in a window too narrow for the list and a conversation
// side by side, such as a half-screen tile: the list fills the space,
// opening a conversation swaps it for the conversation, and closing it
// brings the list back.
import QtQuick
import Quickshell
import "ui"
import "ui/components"
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property var steps: [root.waitForList, root.openFirst, root.checkConversationOnly, root.closeIt, root.checkListOnly]

  // waitForList holds until the demo conversations fill a full-width list
  // and no conversation column is shown.
  function waitForList(): var {
    if (listController.model.count !== 11) return false;
    return root.checkListOnly();
  }

  // checkListOnly checks the list is the only column and fills the space.
  function checkListOnly(): var {
    const listColumn = Check.find(layout, "listColumn");
    const conversation = Check.find(layout, "conversationView");
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
    const listColumn = Check.find(layout, "listColumn");
    const conversation = Check.find(layout, "conversationView");
    if (listColumn.visible) return "the list is still showing beside the open conversation";
    if (!conversation.visible || conversation.width < 400) return "the conversation is not filling the window";
    return true;
  }

  // closeIt closes the conversation again.
  function closeIt(): var {
    conversationController.close();
    return true;
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
    composer: composerController
    photoViewer: photoViewerController
  }

  ComposerController {
    id: composerController

    service: helperService
    conversation: conversationController
  }

  PhotoViewerController {
    id: photoViewerController

    conversation: conversationController
  }

  ReactionsController {
    id: reactionsController

    service: helperService
    conversation: conversationController
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
    composerController: composerController
    photoViewerController: photoViewerController
    reactionsController: reactionsController
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
      composerController: composerController
      photoViewerController: photoViewerController
      reactionsController: reactionsController
      dialogController: dialogController
      windowController: windowController
    }
  }

  // At step 0, waitForList calls checkListOnly() directly once the list
  // fills; a string result there means the "narrow" binding has not
  // settled yet, so it is worth retrying rather than failing outright.
  Stepper {
    id: stepper
    name: "NarrowLayout"
    steps: root.steps
    isFailure: (result) => typeof result === "string" && stepper.step > 0
    startFn: () => stepper.runStep()
  }
}
