// Checks a poll message: it shows its question, each option's text and
// its share of the vote, the signed-in user's own choice marked and not
// by colour alone, a multiple-choice poll's own note, clicking an open
// option reports a vote, and a closed poll takes no clicks.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  readonly property var noAnnotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })
  property var votedCalls: []

  function run(): void {
    if (!root.checkShowsQuestionAndOptions()) return;
    if (!root.checkMarksTheChosenOption()) return;
    if (!root.checkMultipleChoiceNote()) return;
    if (!root.checkClickingAnOptionVotes()) return;
    if (!root.checkClosedPollTakesNoClicks()) return;

    console.log("PASS MessagePoll");
    Qt.exit(0);
  }

  function checkShowsQuestionAndOptions(): bool {
    const question = Check.find(openPoll, "pollQuestion");
    if (!question || question.text !== "Lunch?") return Check.fail("question = " + (question && question.text));

    const pizza = Check.find(openPoll, "pollOptionText-0");
    const salad = Check.find(openPoll, "pollOptionText-1");
    if (!pizza || pizza.text !== "Pizza") return Check.fail("option 0 text = " + (pizza && pizza.text));
    if (!salad || salad.text !== "Salad") return Check.fail("option 1 text = " + (salad && salad.text));

    const share = Check.find(openPoll, "pollOptionShareText-0");
    if (!share || share.text !== "75% · 3") return Check.fail("share text = " + (share && share.text));

    const footer = Check.find(openPoll, "pollFooter");
    if (!footer || footer.text !== "4 votes") return Check.fail("footer = " + (footer && footer.text));
    return true;
  }

  function checkMarksTheChosenOption(): bool {
    const chosenMark = Check.find(openPoll, "pollOptionChosenMark-0");
    if (!chosenMark || !chosenMark.visible) return Check.fail("chosen option has no mark");

    const notChosenMark = Check.find(openPoll, "pollOptionChosenMark-1");
    if (notChosenMark && notChosenMark.visible) return Check.fail("unchosen option shows a chosen mark");

    // Never colour alone: the chosen option also gets a border and a
    // bolder label weight.
    const chosenRow = Check.find(openPoll, "pollOption-0");
    if (chosenRow.border.width <= 0) return Check.fail("chosen option has no outline");
    return true;
  }

  function checkMultipleChoiceNote(): bool {
    const note = Check.find(multiPoll, "pollMultipleChoiceNote");
    if (!note || !note.visible) return Check.fail("multiple-choice poll shows no note");

    const singleNote = Check.find(openPoll, "pollMultipleChoiceNote");
    if (singleNote && singleNote.visible) return Check.fail("single-choice poll shows the multiple-choice note");
    return true;
  }

  function checkClickingAnOptionVotes(): bool {
    root.votedCalls = [];
    t.mouseClick(Check.find(openPoll, "pollOptionArea-1"));
    if (JSON.stringify(root.votedCalls) !== '[["b"]]')
      return Check.fail("clicking Salad reported " + JSON.stringify(root.votedCalls) + ", want [[\"b\"]]");
    return true;
  }

  function checkClosedPollTakesNoClicks(): bool {
    root.votedCalls = [];
    t.mouseClick(Check.find(closedPoll, "pollOptionArea-0"));
    if (root.votedCalls.length !== 0) return Check.fail("a closed poll still reported a vote");
    return true;
  }

  FloatingWindow {
    implicitWidth: 600
    implicitHeight: 600
    visible: true

    Column {
      width: 600

      MessageDelegate {
        id: openPoll
        width: 600
        annotation: root.noAnnotation
        message: ({
          id: "p1", outgoing: false, status: "received", created: 0, senderName: "",
          text: "[Poll: Lunch?]",
          media: { kind: "poll", poll: {
            question: "Lunch?", multipleChoice: false, totalVoters: 4,
            options: [
              { id: "a", text: "Pizza", votes: 3, chosen: true },
              { id: "b", text: "Salad", votes: 1, chosen: false }
            ]
          } }
        })
        onVoted: (id, optionIds) => root.votedCalls.push(optionIds)
      }

      MessageDelegate {
        id: multiPoll
        width: 600
        annotation: root.noAnnotation
        message: ({
          id: "p2", outgoing: false, status: "received", created: 0, senderName: "",
          text: "[Poll: Toppings?]",
          media: { kind: "poll", poll: {
            question: "Toppings?", multipleChoice: true, totalVoters: 0,
            options: [{ id: "a", text: "Olives", votes: 0, chosen: false }]
          } }
        })
      }

      MessageDelegate {
        id: closedPoll
        width: 600
        annotation: root.noAnnotation
        message: ({
          id: "p3", outgoing: false, status: "received", created: 0, senderName: "",
          text: "[Poll: Done?]",
          media: { kind: "poll", poll: {
            question: "Done?", multipleChoice: false, totalVoters: 1, closed: true,
            options: [{ id: "a", text: "Yes", votes: 1, chosen: false }]
          } }
        })
        onVoted: (id, optionIds) => root.votedCalls.push(optionIds)
      }
    }
  }

  TestCase {
    id: t
    when: false
  }

  // Layout settles over a few frames, so measure after a short delay.
  Timer {
    running: true
    interval: 100
    onTriggered: root.run()
  }
}
