import QtQuick
import qs.Commons

// Shared quiet-until-hovered/selected fill colour for a row or list item:
// an accent tint when selected, a plain hover tint when hovered, and
// transparent otherwise. Several rows (palette, mention picker, rail,
// conversation list, new-chat contacts) need exactly this colour, so it
// lives here once rather than drifting out of sync across five files.
QtObject {
  id: root

  // selected marks the row as the current keyboard or list selection.
  property bool selected: false
  // hovered marks the row as under the pointer.
  property bool hovered: false

  // color is the fill this row should paint.
  readonly property color color: root.selected
    ? Util.alpha(Color.accent, Style.selectedFillAlpha)
    : root.hovered ? Style.hoverFill : "transparent"
}
