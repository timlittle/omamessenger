// Checks voting in a poll with real key events, through the same
// routeKey dispatch Panel.qml uses, wired to the real controllers
// against a scripted fake service (not the demo helper, so every
// outcome is deterministic): v opens vote mode on the highlighted
// message's poll, j/k move the highlighted option, Space checks or
// unchecks it for a multiple-choice poll, Enter casts the vote (the
// highlighted option alone when nothing was checked), the poll's
// bubble updates at once before the helper's reply confirms it,
// clicking an option votes directly without opening vote mode first,
// and Escape cancels without voting.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "ui/controllers"
import "ui/lib/Keymap.js" as Keymap
import "ui/lib/Navigation.js" as Navigation
import "ui/lib/Timeline.js" as Timeline
import "Check.js" as Check

ShellRoot {
  id: root

  QtObject {
    id: service

    property var accounts: []
    property var services: []
    property var uiState: ({})
    property var pendingAuth: null
    property string status: "ready"
    property string detail: ""
    property int unreadTotal: 0
    property var votes: []
    readonly property real base: Date.now()

    signal event(string name, var data)

    function start(): void {}
    function installHelper(): void {}
    function quit(): void {}

    function fakeMessage(id: string, created: real, fields: var): var {
      return Object.assign({ id: id, conversationId: "c1", senderId: "s1", senderName: "Alex",
        text: id, outgoing: false, status: "received", created: created }, fields || {});
    }

    // request answers synchronously: a first page with a single-choice
    // poll (m1, newest) and a multiple-choice one (m2, older), and
    // messages.vote echoes back the voted options marked chosen, the
    // way the real helper does.
    function request(method: string, params: var, callback: var): void {
      if (method === "messages.list") {
        callback(null, { hasMore: false, messages: [
          service.fakeMessage("m1", service.base, { media: { kind: "poll", poll: {
            question: "Lunch?", multipleChoice: false, totalVoters: 0,
            options: [{ id: "a", text: "Pizza", votes: 0, chosen: false }, { id: "b", text: "Salad", votes: 0, chosen: false }]
          } } }),
          service.fakeMessage("m2", service.base - 1000, { media: { kind: "poll", poll: {
            question: "Toppings?", multipleChoice: true, totalVoters: 0,
            options: [{ id: "x", text: "Olives", votes: 0, chosen: false }, { id: "y", text: "Mushrooms", votes: 0, chosen: false }]
          } } })
        ] });
        return;
      }

      if (method === "messages.vote") {
        service.votes.push(params);
        callback(null, service.votedMessage(params));
        return;
      }

      callback(null, {});
    }

    // votedMessage answers messages.vote with params.messageId's poll,
    // each requested option marked chosen with one vote.
    function votedMessage(params: var): var {
      const template = params.messageId === "m1"
        ? { question: "Lunch?", multipleChoice: false, options: [{ id: "a", text: "Pizza" }, { id: "b", text: "Salad" }] }
        : { question: "Toppings?", multipleChoice: true, options: [{ id: "x", text: "Olives" }, { id: "y", text: "Mushrooms" }] };
      const options = template.options.map((o) => Object.assign({}, o, {
        votes: params.optionIds.includes(o.id) ? 1 : 0, chosen: params.optionIds.includes(o.id)
      }));
      const created = params.messageId === "m1" ? service.base : service.base - 1000;
      return service.fakeMessage(params.messageId, created, {
        media: { kind: "poll", poll: Object.assign({}, template, { options: options, totalVoters: params.optionIds.length > 0 ? 1 : 0 }) }
      });
    }
  }

  ConversationController {
    id: conversationController
    service: service
  }

  PollsController {
    id: pollsController
    service: service
    conversation: conversationController
  }

  WindowController {
    id: windowController
    service: service
    conversationController: conversationController
    pollsController: pollsController
  }

  function routeKey(key: int, modifiers: int, text: string): bool {
    const context = Navigation.keyContext({
      confirmOpen: false, setupOpen: false, viewerOpen: false, paletteOpen: false,
      reactionPickerOpen: false, pollVoteOpen: pollsController.voteTarget !== "",
      deleteConfirmOpen: false, dialogOpen: false, searchFocused: false,
      composeFocused: false, pane: conversationController.pane
    });
    const action = Keymap.match(context, key, modifiers, text);
    if (!action) return false;

    const controllers = [conversationController, pollsController, windowController];
    const owner = controllers.find((c) => c.handles(action));
    if (!owner) return false;

    owner.run(action);
    return true;
  }

  // pollOf returns the poll media of a loaded message by id.
  function pollOf(id: string): var {
    for (let i = 0; i < conversationController.messages.count; i++) {
      const row = conversationController.messages.get(i);
      if (row.id === id) return Timeline.media(row).poll;
    }
    return null;
  }

  // delegateFor returns id's realized MessageDelegate, or null when the
  // view has not brought it into view, so a check can find an option
  // row inside the right message rather than Check.find's first match
  // across every poll on screen.
  function delegateFor(id: string): var {
    const listView = Check.find(view, "messageListView");
    for (let i = 0; i < listView.count; i++) {
      if (listView.model.get(i).id === id) return listView.itemAtIndex(i);
    }
    return null;
  }

  FloatingWindow {
    id: win
    implicitWidth: 500
    implicitHeight: 400
    visible: true

    Item {
      id: keyArea
      anchors.fill: parent
      focus: true

      Keys.onPressed: event => {
        if (root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
      }

      ConversationView {
        id: view
        anchors.fill: parent
        conversation: conversationController.conversation
        messages: conversationController.messages
        annotations: conversationController.annotations
        highlightedId: conversationController.highlightedId
        voteState: ({
          target: pollsController.voteTarget, highlightedIndex: pollsController.voteIndex, selectedIds: pollsController.selected
        })
        composeEnabled: true
        routeKey: root.routeKey

        onVoted: (id, optionIds) => pollsController.castVote(id, optionIds)
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
    onTriggered: root.start()
  }

  function start(): void {
    conversationController.open({ id: "c1", accountId: "a1", service: "whatsapp", remoteId: "r1",
      kind: "direct", title: "Alex", members: 0, preview: "", muted: false, unread: 0, lastActivity: Date.now() });
    root.checkSingleChoiceVoteByKeyboard();
  }

  // checkSingleChoiceVoteByKeyboard opens vote mode on m1, moves to its
  // second option and casts it with Enter alone, with nothing checked
  // first.
  function checkSingleChoiceVoteByKeyboard(): void {
    if (conversationController.highlightedId !== "m1") return Check.fail("setup: m1 is not highlighted");

    t.keyClick(Qt.Key_V);
    if (pollsController.voteTarget !== "m1" || pollsController.voteIndex !== 0)
      return Check.fail("v did not open vote mode on m1 at option 0");

    t.keyClick(Qt.Key_J);
    if (pollsController.voteIndex !== 1) return Check.fail("j did not move to option 1");

    t.keyClick(Qt.Key_Return);
    // Casting a vote updates the message's media, which the ListView
    // re-lays out for; letting that settle before the next synthetic
    // key event avoids a spurious "window not shown" from Qt's own test
    // input delivery landing mid-layout.
    t.wait(50);
    if (JSON.stringify(service.votes) !== '[{"messageId":"m1","optionIds":["b"]}]')
      return Check.fail("Enter did not cast Salad: " + JSON.stringify(service.votes));
    if (pollsController.voteTarget !== "") return Check.fail("vote mode did not close after casting");

    const poll = root.pollOf("m1");
    if (!poll || !poll.options[1].chosen) return Check.fail("m1's bubble did not update with the cast vote");

    root.checkMultipleChoiceVoteByKeyboard();
  }

  // checkMultipleChoiceVoteByKeyboard opens vote mode on m2, checks both
  // options with Space, then casts with Enter.
  function checkMultipleChoiceVoteByKeyboard(): void {
    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m2") return Check.fail("setup: k did not reach m2");

    t.keyClick(Qt.Key_V);
    if (pollsController.voteTarget !== "m2") return Check.fail("v did not open vote mode on m2");

    t.keyClick(Qt.Key_Space);
    if (JSON.stringify(pollsController.selected) !== '["x"]') return Check.fail("Space did not check Olives");

    t.keyClick(Qt.Key_J);
    t.keyClick(Qt.Key_Space);
    if (JSON.stringify(pollsController.selected) !== '["x","y"]') return Check.fail("Space did not check Mushrooms too");

    service.votes = [];
    t.keyClick(Qt.Key_Return);
    t.wait(50);
    if (JSON.stringify(service.votes) !== '[{"messageId":"m2","optionIds":["x","y"]}]')
      return Check.fail("Enter did not cast both checked options: " + JSON.stringify(service.votes));

    root.checkEscapeCancels();
  }

  // checkEscapeCancels opens vote mode again and checks Escape leaves it
  // without casting anything.
  function checkEscapeCancels(): void {
    t.keyClick(Qt.Key_V);
    if (pollsController.voteTarget !== "m2") return Check.fail("setup: v did not reopen vote mode");

    service.votes = [];
    t.keyClick(Qt.Key_Escape);
    if (pollsController.voteTarget !== "") return Check.fail("Escape did not cancel vote mode");
    if (service.votes.length !== 0) return Check.fail("Escape cast a vote");

    root.checkMouseClickVotesDirectly();
  }

  // checkMouseClickVotesDirectly clicks Pizza on m1 directly, with vote
  // mode never opened first, the way a mouse user votes.
  function checkMouseClickVotesDirectly(): void {
    service.votes = [];
    t.mouseClick(Check.find(root.delegateFor("m1"), "pollOptionArea-0"));
    t.wait(50);
    if (JSON.stringify(service.votes) !== '[{"messageId":"m1","optionIds":["a"]}]')
      return Check.fail("clicking Pizza did not cast a vote: " + JSON.stringify(service.votes));

    console.log("PASS PollVote");
    Qt.exit(0);
  }
}
