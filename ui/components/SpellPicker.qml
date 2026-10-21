pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import "../theme"

// The spelling suggestions picker: up to five replacement words for the
// misspelled word under the caret, plus "Add to dictionary" and
// "Ignore", shown above the composer's text field while it is open (see
// Keymap.js's spellPicker context). Up/Down move the keyboard
// highlight; clicking a row reaches the same effect by mouse. It takes
// data through properties and reports the choice through a signal, same
// as MentionPicker: Composer.qml owns the key routing that drives
// highlightedIndex.
Rectangle {
  id: root

  // word is the misspelled word the picker is open for, shown as a
  // heading so it is clear which word a suggestion would replace.
  property string word: ""
  // suggestions are the replacement words to offer.
  property var suggestions: []
  // highlightedIndex is which row the keyboard currently points at,
  // indexing suggestions first and then the two action rows below them.
  property int highlightedIndex: 0

  // rowCount is how many rows the picker shows in total: one per
  // suggestion, plus "Add to dictionary" and "Ignore".
  readonly property int rowCount: root.suggestions.length + 2

  // suggestionChosen reports a suggestion at index was picked.
  signal suggestionChosen(int index)
  // addToDictionaryRequested reports "Add to dictionary" was chosen.
  signal addToDictionaryRequested()
  // ignoreRequested reports "Ignore" was chosen.
  signal ignoreRequested()

  objectName: "spellPicker"
  implicitWidth: Style.space(220)
  implicitHeight: column.implicitHeight + Theme.spacing.xxs * 2
  radius: Style.cornerRadius
  color: Style.normalFill
  border.width: Theme.spacing.hairline
  border.color: Util.alpha(Theme.foreground, 0.15)
  clip: true

  Column {
    id: column
    y: Theme.spacing.xxs
    width: parent.width

    Text {
      objectName: "spellPickerWord"
      x: Theme.spacing.sm
      width: parent.width - Theme.spacing.sm * 2
      text: root.word
      // The flagged word comes from the message being typed: PlainText.
      textFormat: Text.PlainText
      elide: Text.ElideRight
      color: Util.alpha(Theme.foreground, 0.6)
      font.family: Theme.font.family
      font.pixelSize: Theme.font.caption
    }

    Repeater {
      model: root.suggestions

      Rectangle {
        id: row
        required property string modelData
        required property int index

        readonly property bool current: row.index === root.highlightedIndex

        objectName: "spellSuggestionRow-" + row.index
        width: column.width
        height: label.implicitHeight + Theme.spacing.xs * 2
        color: fill.color

        HighlightFill {
          id: fill
          selected: row.current
          hovered: hover.containsMouse
        }

        Text {
          id: label
          anchors.verticalCenter: parent.verticalCenter
          x: Theme.spacing.sm
          text: row.modelData
          // A suggested word is derived from the dictionary, not shown
          // text from outside, but PlainText anyway: it is never markup.
          textFormat: Text.PlainText
          color: Theme.foreground
          font.family: Theme.font.family
          font.pixelSize: Theme.font.body
        }

        MouseArea {
          id: hover
          objectName: "spellSuggestionRowArea-" + row.index
          anchors.fill: parent
          hoverEnabled: true
          cursorShape: Qt.PointingHandCursor
          onClicked: root.suggestionChosen(row.index)
        }
      }
    }

    Rectangle {
      id: addRow
      readonly property bool current: root.suggestions.length === root.highlightedIndex

      objectName: "spellAddToDictionaryRow"
      width: column.width
      height: addLabel.implicitHeight + Theme.spacing.xs * 2
      color: addFill.color

      HighlightFill {
        id: addFill
        selected: addRow.current
        hovered: addHover.containsMouse
      }

      Text {
        id: addLabel
        anchors.verticalCenter: parent.verticalCenter
        x: Theme.spacing.sm
        text: "Add to dictionary"
        textFormat: Text.PlainText
        color: Theme.foreground
        font.family: Theme.font.family
        font.pixelSize: Theme.font.body
      }

      MouseArea {
        id: addHover
        objectName: "spellAddToDictionaryArea"
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: root.addToDictionaryRequested()
      }
    }

    Rectangle {
      id: ignoreRow
      readonly property bool current: root.suggestions.length + 1 === root.highlightedIndex

      objectName: "spellIgnoreRow"
      width: column.width
      height: ignoreLabel.implicitHeight + Theme.spacing.xs * 2
      color: ignoreFill.color

      HighlightFill {
        id: ignoreFill
        selected: ignoreRow.current
        hovered: ignoreHover.containsMouse
      }

      Text {
        id: ignoreLabel
        anchors.verticalCenter: parent.verticalCenter
        x: Theme.spacing.sm
        text: "Ignore"
        textFormat: Text.PlainText
        color: Theme.foreground
        font.family: Theme.font.family
        font.pixelSize: Theme.font.body
      }

      MouseArea {
        id: ignoreHover
        objectName: "spellIgnoreArea"
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: root.ignoreRequested()
      }
    }
  }
}
