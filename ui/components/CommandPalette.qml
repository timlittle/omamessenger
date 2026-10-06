pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// The command palette: type to filter, Enter to run. It lists commands
// with their shortcut beside them, so people learn the keys as they go,
// or conversations to jump to. What it lists is up to the caller.
Item {
  id: root

  // open shows the palette when true.
  property bool open: false
  // placeholder is the search field's hint, naming what is listed.
  property string placeholder: "Type a command"
  // items are the rows to show: {label, detail, keys}.
  property var items: []
  // currentIndex is the highlighted row.
  property int currentIndex: 0
  // routeKey is the panel's key router, so the palette's keys work while
  // the search field has focus.
  property var routeKey: null

  // queryEdited reports the search text as it is typed.
  signal queryEdited(string text)
  // accepted runs the row at index.
  signal accepted(int index)
  // cancelled closes the palette.
  signal cancelled()

  // focusInput puts keyboard focus in the search field.
  function focusInput(): void {
    field.forceActiveFocus();
  }

  objectName: "commandPalette"
  anchors.fill: parent
  visible: root.open
  onOpenChanged: {
    if (!root.open) return;

    field.text = "";
    root.focusInput();
  }

  Rectangle {
    anchors.fill: parent
    color: Theme.menu.scrim

    MouseArea {
      anchors.fill: parent
      onClicked: root.cancelled()
    }
  }

  // Near the top of the window, like Telescope or Alfred, so the list has
  // room to grow downwards.
  Ui.BorderSurface {
    id: card

    width: Math.min(parent.width - Theme.spacing.xxl * 2, Style.space(640))
    height: Math.min(parent.height - Theme.spacing.xxl * 2, field.implicitHeight + list.contentHeight
      + Theme.spacing.md + card.contentTopInset + card.contentBottomInset)
    color: Theme.popups.background
    borderSpec: Border.flat(Theme.popups.border, Style.normalBorderWidth)
    radius: Style.cornerRadius
    padding: Theme.spacing.panelPadding
    anchors { horizontalCenter: parent.horizontalCenter; top: parent.top; topMargin: Theme.spacing.xxl * 2 }

    MouseArea {
      anchors.fill: parent
      onClicked: () => {}
    }

    ColumnLayout {
      spacing: Theme.spacing.md
      anchors {
        fill: parent
        topMargin: card.contentTopInset
        rightMargin: card.contentRightInset
        bottomMargin: card.contentBottomInset
        leftMargin: card.contentLeftInset
      }

      Ui.TextField {
        id: field

        objectName: "paletteField"
        Layout.fillWidth: true
        placeholderText: root.placeholder
        Keys.priority: Keys.BeforeItem
        Keys.onPressed: event => {
          if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true;
        }
        onTextChanged: root.queryEdited(text)
      }

      ListView {
        id: list

        objectName: "paletteList"
        Layout.fillWidth: true
        Layout.fillHeight: true
        clip: true
        model: root.items
        currentIndex: root.currentIndex
        onCurrentIndexChanged: list.positionViewAtIndex(list.currentIndex, ListView.Contain)

        delegate: PaletteRow {
          required property var modelData
          required property int index

          width: ListView.view.width
          item: modelData
          current: index === root.currentIndex
          onChosen: root.accepted(index)
        }
      }

      Text {
        visible: root.items.length === 0
        text: "No matches"
        color: Util.alpha(Color.foreground, 0.5)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }
    }
  }
}
