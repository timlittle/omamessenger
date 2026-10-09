pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Poll.js" as Poll

// A poll in a message: its question, each option as a row with a bar
// showing its share of the vote and the raw count, the signed-in
// user's own choice marked with a glyph and bolder weight (never colour
// alone), and a multiple-choice note when more than one pick is
// allowed. A closed poll shows the same bars but takes no clicks and
// enters no vote mode. Voting itself has two ways in: clicking a row
// casts that single choice outright, or the caller puts this message
// into vote mode (see PollsController) and drives highlightedIndex and
// selectedIds from the keyboard, which this view only ever reflects.
Item {
  id: root

  // poll is the message's poll media: question, options (each
  // {id, text, votes, chosen, correct}), multipleChoice, quiz, closed
  // and totalVoters.
  required property var poll
  // voting is true while this message is in vote mode, highlighting
  // highlightedIndex and outlining selectedIds instead of just chosen.
  property bool voting: false
  // highlightedIndex is the keyboard-highlighted option while voting.
  property int highlightedIndex: -1
  // selectedIds are the option ids checked so far while voting.
  property var selectedIds: []

  // voted asks the caller to cast optionIds as the user's vote, from a
  // direct click on one option.
  signal voted(var optionIds)

  // _shares is each option's percentage of the vote, in option order.
  readonly property var _shares: Poll.percentages(root.poll)

  implicitWidth: Style.space(280)
  implicitHeight: column.implicitHeight

  ColumnLayout {
    id: column

    anchors { left: parent.left; right: parent.right }
    spacing: Theme.spacing.xs

    Text {
      Layout.fillWidth: true
      objectName: "pollQuestion"
      text: root.poll.question
      // A poll's question and options are written by whoever sent it:
      // PlainText so "<img src=...>" is never auto-detected as rich
      // text and never fetches anything.
      textFormat: Text.PlainText
      wrapMode: Text.Wrap
      color: Color.foreground
      font { family: Theme.font.family; pixelSize: Theme.font.body; weight: Font.DemiBold }
    }

    Text {
      Layout.fillWidth: true
      objectName: "pollMultipleChoiceNote"
      visible: !!root.poll.multipleChoice
      text: "Select one or more"
      textFormat: Text.PlainText
      color: Util.alpha(Color.foreground, 0.6)
      font { family: Theme.font.family; pixelSize: Theme.font.caption }
    }

    Repeater {
      model: root.poll.options || []

      Rectangle {
        id: optionRow

        required property var modelData
        required property int index

        readonly property bool checked: root.voting ? root.selectedIds.includes(optionRow.modelData.id) : optionRow.modelData.chosen
        readonly property bool rowHighlighted: root.voting && root.highlightedIndex === optionRow.index

        objectName: "pollOption-" + optionRow.index
        Layout.fillWidth: true
        implicitHeight: label.implicitHeight + Theme.spacing.sm * 2
        radius: Style.cornerRadius
        color: optionRow.rowHighlighted ? Style.hoverFill : optionHover.containsMouse ? Style.hoverFill : "transparent"
        border.width: optionRow.checked ? Theme.spacing.hairline : 0
        border.color: Color.accent
        clip: true

        // share is this option's bar, drawn behind its text and count.
        Rectangle {
          objectName: "pollOptionShare-" + optionRow.index
          anchors { left: parent.left; top: parent.top; bottom: parent.bottom }
          width: parent.width * (root._shares[optionRow.index] ?? 0) / 100
          color: Util.alpha(Color.accent, 0.14)
        }

        RowLayout {
          id: label

          anchors { left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter }
          anchors { leftMargin: Theme.spacing.sm; rightMargin: Theme.spacing.sm }
          spacing: Theme.spacing.xs

          Text {
            objectName: "pollOptionChosenMark-" + optionRow.index
            visible: optionRow.checked
            text: "✓"
            textFormat: Text.PlainText
            color: Color.accent
            font { family: Theme.font.family; pixelSize: Theme.font.body }
          }

          Text {
            Layout.fillWidth: true
            objectName: "pollOptionText-" + optionRow.index
            text: optionRow.modelData.text
            // Each option's own text, also sender-controlled: PlainText.
            textFormat: Text.PlainText
            wrapMode: Text.Wrap
            color: Color.foreground
            font { family: Theme.font.family; pixelSize: Theme.font.body; weight: optionRow.checked ? Font.DemiBold : Font.Normal }
          }

          Text {
            objectName: "pollOptionShareText-" + optionRow.index
            text: (root._shares[optionRow.index] ?? 0) + "% · " + optionRow.modelData.votes
            textFormat: Text.PlainText
            color: Util.alpha(Color.foreground, 0.6)
            font { family: Theme.font.family; pixelSize: Theme.font.caption }
          }
        }

        MouseArea {
          id: optionHover

          objectName: "pollOptionArea-" + optionRow.index
          anchors.fill: parent
          hoverEnabled: !root.poll.closed
          cursorShape: root.poll.closed ? Qt.ArrowCursor : Qt.PointingHandCursor
          onClicked: if (!root.poll.closed) root.voted([optionRow.modelData.id])
        }
      }
    }

    Text {
      Layout.fillWidth: true
      objectName: "pollFooter"
      text: (root.poll.closed ? "Closed · " : "") + Poll.voterLabel(root.poll.totalVoters)
      textFormat: Text.PlainText
      color: Util.alpha(Color.foreground, 0.5)
      font { family: Theme.font.family; pixelSize: Theme.font.caption }
    }
  }
}
