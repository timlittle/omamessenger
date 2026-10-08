pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// The shared frame every yes/no/choice question in the window builds on:
// a title, an optional description, a routeKey header and a list of
// labelled buttons with a highlighted default. RemoveAccount,
// DeleteConfirm, ArchiveAllConfirm and CloseConfirm each set the title,
// description and choices for their own question; AccountSetup is
// different enough (multiple stages, text fields) that it stays its own
// component.
Item {
  id: root

  // open shows the dialog.
  property bool open: false
  // title is the question, shown bold at the top.
  property string title: ""
  // titleObjectName names the title Text for a test that measures it
  // directly; empty leaves it unnamed.
  property string titleObjectName: ""
  // description is shown under the title, wrapped; empty hides it.
  property string description: ""
  // errorText is shown under the main choices, in the urgent colour, for
  // the "list" layout only; empty hides it.
  property string errorText: ""
  // choices are the main list of buttons, left to right for "row" or top
  // to bottom for "list": each one is {objectName, text, selected, value}
  // plus an optional visible, which defaults to true when left out.
  property var choices: []
  // trailingChoices are extra buttons shown after errorText, in the
  // "list" layout only; only RemoveAccount uses this, for its separate
  // Cancel button below the account list.
  property var trailingChoices: []
  // layout is "row" (buttons right-aligned on one line, the shape every
  // confirm dialog but RemoveAccount uses) or "list" (a fill-width
  // vertical list, left-aligned, for RemoveAccount's variable-length
  // account list).
  property string layout: "row"
  // minCardWidth is the card's minimum width; "row" widens it to fit the
  // buttons, "list" uses it as the card's width outright.
  property real minCardWidth: Style.space(420)
  // routeKey intercepts a key press before this dialog's own control
  // handles it; see Composer.qml for why this is a function property, not
  // a signal.
  property var routeKey: null

  // picked reports the value of whichever choice was clicked, or
  // "cancel" for a click outside the card. The dialog built on this maps
  // "cancel" to its own cancelled() signal and any other value to its own
  // named signal.
  signal picked(string value)

  // _choiceAt returns the choice at index, or an empty placeholder while
  // choices is still catching up with a model count change.
  function _choiceAt(list: var, index: int): var {
    return list[index] || {};
  }

  // _isVisible reports whether a choice should show: true when it leaves
  // "visible" out.
  function _isVisible(choice: var): bool {
    return choice.visible === undefined ? true : choice.visible;
  }

  anchors.fill: parent
  visible: root.open
  focus: root.open

  Keys.onPressed: event => {
    if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
  }

  ModalCard {
    id: modal

    cardWidth: root.layout === "row" ? Math.max(root.minCardWidth, rowButtons.implicitWidth + modal.horizontalInsets) : root.minCardWidth
    onOutsideClicked: root.picked("cancel")

    ColumnLayout {
      anchors.fill: parent
      spacing: Theme.spacing.md

      Text {
        objectName: root.titleObjectName
        text: root.title
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        visible: root.description !== ""
        text: root.description
        wrapMode: Text.WordWrap
        color: Util.alpha(Color.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }

      RowLayout {
        id: rowButtons

        visible: root.layout === "row"
        Layout.alignment: Qt.AlignRight
        spacing: Theme.spacing.controlGap

        Repeater {
          model: root.layout === "row" ? root.choices.length : 0

          Ui.Button {
            required property int index
            readonly property var choice: root._choiceAt(root.choices, index)

            objectName: choice.objectName || ""
            text: choice.text || ""
            visible: root._isVisible(choice)
            selected: !!choice.selected
            onClicked: root.picked(choice.value)
          }
        }
      }

      Repeater {
        model: root.layout === "list" ? root.choices.length : 0

        Ui.Button {
          required property int index
          readonly property var choice: root._choiceAt(root.choices, index)

          objectName: choice.objectName || ""
          Layout.fillWidth: true
          leftAlign: true
          text: choice.text || ""
          visible: root._isVisible(choice)
          selected: !!choice.selected
          onClicked: root.picked(choice.value)
        }
      }

      Text {
        Layout.fillWidth: true
        visible: root.layout === "list" && root.errorText !== ""
        text: root.errorText
        wrapMode: Text.WordWrap
        color: Color.urgent
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
      }

      Repeater {
        model: root.layout === "list" ? root.trailingChoices.length : 0

        Ui.Button {
          required property int index
          readonly property var choice: root._choiceAt(root.trailingChoices, index)

          objectName: choice.objectName || ""
          Layout.alignment: Qt.AlignRight
          text: choice.text || ""
          visible: root._isVisible(choice)
          selected: !!choice.selected
          onClicked: root.picked(choice.value)
        }
      }
    }
  }
}
