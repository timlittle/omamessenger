// Checks that the search field and the composer's text input are each
// destroyed, not merely hidden, exactly while they are irrelevant - a
// hidden-but-alive TextInput still reads Wayland clipboard changes with
// no size or time limit, the same risk tests/unit/textinput-lifetime
// guards for every other field in this UI (see docs/decisions.md) - and
// that both still work normally whenever they are shown. Drives a
// narrow window (list and conversation never both fit) the same way
// NarrowLayout does: the search field exists only while the list column
// shows, and the composer's input exists only while a conversation is
// open and its column shows, with neither ever both at once in a narrow
// window.
import QtQuick
import Quickshell
import "ui"
import "ui/components"
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property var steps: [
    root.waitForList,
    root.checkListOnlyFields,
    root.checkSearchStillWorks,
    root.openFirst,
    root.checkConversationOnlyFields,
    root.checkComposeStillWorks,
    root.closeIt,
    root.checkSearchSurvivedHiding
  ]

  // waitForList holds until the demo conversations fill a full-width list.
  function waitForList(): var {
    return listController.model.count === 11;
  }

  // checkListOnlyFields verifies the search field exists while the list
  // column shows, and the composer's own input does not: no conversation
  // is open, and the conversation column is hidden besides.
  function checkListOnlyFields(): var {
    if (!layout.narrow) return "a 600-wide window is not treated as narrow";
    if (!Check.find(layout, "searchField")) return "searchField does not exist while the list column shows";
    if (Check.find(layout, "composerInput")) return "composerInput exists with no conversation open";
    return true;
  }

  // checkSearchStillWorks types into the search field as a user would
  // and confirms it still reaches ListController, the same way it did
  // before the field was gated behind a Loader.
  function checkSearchStillWorks(): var {
    const field = Check.find(layout, "searchField");
    field.text = "dentist";
    if (listController.query !== "dentist") return "typing into the search field did not reach ListController.query";
    return true;
  }

  // openFirst opens the first conversation, which in a narrow window
  // swaps the list column for the conversation column.
  function openFirst(): var {
    conversationController.open(listController.model.get(0));
    return true;
  }

  // checkConversationOnlyFields verifies the reverse of
  // checkListOnlyFields: the composer's input now exists, and the
  // search field does not, since the list column is hidden.
  function checkConversationOnlyFields(): var {
    if (Check.find(layout, "searchField")) return "searchField still exists with the list column hidden";
    if (!Check.find(layout, "composerInput")) return "composerInput does not exist with a conversation open";
    return true;
  }

  // checkComposeStillWorks types into the composer's input as a user
  // would and confirms it still reaches ComposerController's own draft,
  // the same way it did before the input was gated behind a Loader.
  function checkComposeStillWorks(): var {
    const input = Check.find(layout, "composerInput");
    input.text = "on my way";
    if (composerController.draft !== "on my way") return "typing into the composer did not reach ComposerController.draft";
    return true;
  }

  // closeIt closes the conversation again, swapping the conversation
  // column back for the list column.
  function closeIt(): var {
    conversationController.close();
    return true;
  }

  // checkSearchSurvivedHiding verifies the search field is back, with
  // the text it held before it was destroyed: the controller, not the
  // field itself, is what actually held it meanwhile.
  function checkSearchSurvivedHiding(): var {
    const field = Check.find(layout, "searchField");
    if (!field) return "searchField did not come back once the list column showed again";
    if (field.text !== "dentist") return "search field text \"" + field.text + "\" did not survive being hidden and shown again";
    if (Check.find(layout, "composerInput")) return "composerInput still exists once the conversation column hid again";

    console.log("PASS FieldLifetime");
    Qt.exit(0);
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

  // At step 0, waitForList is a plain boolean (no "narrow" binding to
  // settle first, unlike NarrowLayout's own first step), so no
  // isFailure override is needed here.
  Stepper {
    id: stepper
    name: "FieldLifetime"
    steps: root.steps
    startFn: () => stepper.runStep()
  }
}
