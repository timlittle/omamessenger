pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Limits.js" as Limits

// The command palette: type to filter, Enter to run. It lists commands
// with their shortcut beside them, so people learn the keys as they go,
// or conversations to jump to. What it lists is up to the caller.
Item {
  id: root

  // open shows the palette when true.
  property bool open: false
  // placeholder is the search field's hint, naming what is listed.
  property string placeholder: "Type a command"
  // items are the rows to show: {label, detail, keys}. A row may also
  // carry section ("conversation" or "message"), shown under its own
  // heading when sectioned is true: the unified Ctrl+K palette's own
  // "Conversations" and "Messages" lists.
  property var items: []
  // sectioned groups items by their own section field, with a heading
  // for each one that has at least one row.
  property bool sectioned: false
  // currentIndex is the highlighted row.
  property int currentIndex: 0
  // routeKey is the panel's key router, so the palette's keys work while
  // the search field has focus.
  property var routeKey: null
  // windowActive is whether the user can see the window at all: shown
  // and focused. The search field exists only while this and open are
  // both true (see fieldLoader below): a field merely visible while the
  // window sits unfocused in the background - alt-tabbed away - is
  // still a live sink for a foreign Wayland clipboard offer, with no
  // size or time limit (see docs/decisions.md). Defaults true so a
  // caller or test that never sets it (tests/qml/FieldPaste, say) keeps
  // the field exactly as it was before this existed.
  property bool windowActive: true

  // queryEdited reports the search text as it is typed.
  signal queryEdited(string text)
  // accepted runs the row at index.
  signal accepted(int index)
  // cancelled closes the palette.
  signal cancelled()

  // _query is the field's own typed text, kept here rather than only in
  // the field itself: fieldLoader below destroys and recreates it
  // across a window-focus blip, so something outside it has to be the
  // value's real home, the same reasoning Composer.qml's own text
  // documents. It always starts "" since a fresh palette session (a new
  // instance - see Overlays.qml) always starts with nothing typed.
  property string _query: ""
  // _fieldHadFocus is true from the moment the real field last gained
  // activeFocus until it loses it for a reason other than the window
  // itself going inactive (a real blur: Escape, Tab, a click
  // elsewhere) - see the field's own onActiveFocusChanged below.
  // windowActive turning true again only puts focus back in the field
  // while this is also true, rather than stealing it from whatever the
  // user had instead moved it to before switching away.
  property bool _fieldHadFocus: false
  // _fieldHeight remembers the search field's own implicitHeight the
  // last time it existed, so the card's own height stays stable across
  // a window-focus blip destroying and recreating it: fieldLoader.item
  // itself is null for exactly that gap.
  property real _fieldHeight: 0
  // field exposes the loaded search field itself, or null while the
  // window is inactive and it does not exist. A plain var, not an
  // alias, since the Loader's own item has no declared type for a
  // static checker to verify members against (the same reasoning
  // VoiceNoteController's own `_engine` documents).
  property var field: fieldLoader.item

  // focusInput puts keyboard focus in the search field, if it exists.
  function focusInput(): void {
    if (root.field) root.field.forceActiveFocus();
  }

  objectName: "commandPalette"
  anchors.fill: parent
  visible: root.open
  // Regaining the window's focus brings the field back only if it was
  // the thing actually focused when that focus was lost; see
  // _fieldHadFocus above. Deferred with Qt.callLater, so by the time
  // this runs, fieldLoader's own active binding has already recreated
  // the item this focuses (see AccountSetup.qml's own _focusStep for
  // the same reasoning).
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

      // The search field is destroyed, not merely hidden, exactly while
      // the window is inactive: a merely-visible, unfocused TextField
      // still reads every foreign Wayland clipboard offer, with no size
      // or time limit, for however long an alt-tab lasts (see
      // windowActive above and docs/decisions.md). _query is what the
      // field is seeded from and mirrors every keystroke into, so
      // recreating it on return loses nothing the user typed.
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
          // Qt's own right-click "Paste" menu would read the clipboard
          // in-process, with no size or time limit of its own; routeKey
          // above already routes Ctrl+V and Shift+Insert through the
          // helper's bounded clipboard.readText instead (see
          // docs/decisions.md), so this menu is removed outright rather
          // than left to offer the one path around that.
          ContextMenu.menu: null
          Keys.priority: Keys.BeforeItem
          Keys.onPressed: event => {
            // Qt's own paste shortcut is more than Ctrl+V and Shift+Insert
            // (Ctrl+Shift+Insert and the dedicated Paste key on some
            // themes too); catching it here, ahead of the binding lookup
            // routeKey does below, means a rebound field.paste in
            // keys.conf still can never bring back this field's own
            // unbounded native paste() (see docs/decisions.md).
            if (event.matches(StandardKey.Paste) || event.key === Qt.Key_Paste) {
              if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
              event.accepted = true;
              return;
            }
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true;
          }
          onTextChanged: { root._query = text; root.queryEdited(text); }
          // Losing focus because the window itself went inactive is not
          // a real blur: root.windowActive is already false by the time
          // this runs, since that is what emptied fieldLoader.active in
          // the first place, so _fieldHadFocus is left alone for that
          // case and only cleared for an actual Escape, Tab or outside
          // click.
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
