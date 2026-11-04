pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Limits.js" as Limits
import "../lib/Paste.js" as Paste

// The command palette: type to filter, Enter to run. It lists commands
// with their shortcut beside them, so people learn the keys as they go,
// or conversations to jump to. What it lists is up to the caller.
Item {
  id: root

  // open shows the palette when true.
  property bool open: false
  // placeholder is the search field's hint, naming what is listed.
  property string placeholder: "Type a command"
  // items are the rows to show: {label, detail, keys}, optionally with a section name.
  property var items: []
  // sectioned groups items by their own section field, with a heading
  // for each one that has at least one row.
  property bool sectioned: false
  // currentIndex is the highlighted row.
  property int currentIndex: 0
  // routeKey is the panel's key router, so the palette's keys work while
  // the search field has focus.
  property var routeKey: null
  // windowActive is whether the window is shown and focused. The
  // search field exists only while this and open are both true (see
  // fieldLoader): an unfocused field still reads a foreign clipboard
  // offer unbounded otherwise (see docs/decisions.md).
  property bool windowActive: true

  // queryEdited reports the search text as it is typed.
  signal queryEdited(string text)
  // accepted runs the row at index.
  signal accepted(int index)
  // cancelled closes the palette.
  signal cancelled()

  // _query is the field's typed text, kept here since fieldLoader
  // destroys and recreates the field across a window-focus blip.
  property string _query: ""
  // _fieldHadFocus is true from when the field last gained focus until
  // a real blur (Escape, Tab, a click elsewhere), not a window-inactive teardown.
  property bool _fieldHadFocus: false
  // _fieldHeight remembers the field's implicitHeight so the card's
  // height stays stable while fieldLoader.item is null across a blip.
  property real _fieldHeight: 0
  // field exposes the loaded search field, or null while inactive. A
  // plain var since the Loader's item has no declared type.
  property var field: fieldLoader.item

  // focusInput puts keyboard focus in the search field, if it exists.
  function focusInput(): void {
    if (root.field) root.field.forceActiveFocus();
  }

  objectName: "commandPalette"
  anchors.fill: parent
  visible: root.open
  // Regains focus only if the field had it when focus was lost.
  // Deferred with Qt.callLater so fieldLoader has already recreated the item.
  onWindowActiveChanged: if (root.windowActive && root._fieldHadFocus) Qt.callLater(root.focusInput);

  // Near the top of the window, like Telescope or Alfred, so the list has
  // room to grow downwards.
  ModalCard {
    id: modal

    cardWidth: Style.space(640)
    cardHeight: root._fieldHeight + list.contentHeight + Theme.spacing.md + modal.verticalInsets
    atTop: true
    onOutsideClicked: root.cancelled()

    ColumnLayout {
      anchors.fill: parent
      spacing: Theme.spacing.md

      // Destroyed, not merely hidden, while the window is inactive (see
      // windowActive); _query preserves what was typed across that gap.
      Loader {
        id: fieldLoader

        Layout.fillWidth: true
        active: root.open && root.windowActive
        onLoaded: {
          root._fieldHeight = root.field.implicitHeight;
          if (root.open) root.field.forceActiveFocus();
        }

        sourceComponent: Ui.TextField {
          objectName: "paletteField"
          text: root._query
          placeholderText: root.placeholder
          maximumLength: Limits.PALETTE_QUERY
          // Qt's right-click "Paste" menu reads the clipboard
          // unbounded; routeKey already routes paste through the
          // helper's bounded path instead (see docs/decisions.md).
          ContextMenu.menu: null
          Keys.priority: Keys.BeforeItem
          Keys.onPressed: event => {
            // Caught ahead of routeKey so a rebound field.paste in
            // keys.conf can't bring back the unbounded native paste().
            if (Paste.isPasteKey(event.key, event.modifiers)) {
              if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
              event.accepted = true;
              return;
            }
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true;
          }
          onTextChanged: { root._query = text; root.queryEdited(text); }
          // A window-inactive teardown is not a real blur, so _fieldHadFocus is left alone for it.
          onActiveFocusChanged: {
            if (activeFocus) { root._fieldHadFocus = true; return; }
            if (root.windowActive) root._fieldHadFocus = false;
          }

          MiddleClickGuard {}
        }
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

        section.property: root.sectioned ? "section" : ""
        section.delegate: Component {
          Text {
            required property string section

            text: section === "message" ? "Messages" : "Conversations"
            textFormat: Text.PlainText
            topPadding: Theme.spacing.sm
            bottomPadding: Theme.spacing.xs
            color: Util.alpha(Theme.foreground, 0.5)
            font { family: Theme.font.family; pixelSize: Theme.font.caption; bold: true }
          }
        }

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
        textFormat: Text.PlainText
        color: Util.alpha(Theme.foreground, 0.5)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }
    }
  }
}
