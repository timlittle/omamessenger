import QtQuick
import qs.Commons
import qs.Ui as Ui
import "../theme"

// The frame every dialog in the window shares: the window dimmed behind a
// card with the popup border and padding. The dialog's content goes inside
// as one child that fills the card; clicks outside the card are reported,
// and never reach what is behind it.
Item {
  id: root

  // cardWidth is the card's width, narrowed to fit the window.
  property real cardWidth: Style.space(420)
  // cardHeight is the card's height, or 0 to fit the content.
  property real cardHeight: 0
  // atTop places the card near the top of the window instead of centring it.
  property bool atTop: false
  // cardName is the card's objectName, for tests that measure it.
  property string cardName: ""
  // horizontalInsets is the border and padding either side of the content.
  readonly property real horizontalInsets: card.contentLeftInset + card.contentRightInset
  // verticalInsets is the border and padding above and below the content.
  readonly property real verticalInsets: card.contentTopInset + card.contentBottomInset
  // content is the dialog inside the card.
  default property alias content: inner.data

  // outsideClicked reports a click on the dimmed window around the card.
  signal outsideClicked()

  // _contentHeight is the content's natural height.
  function _contentHeight(): real {
    return inner.children.length > 0 ? inner.children[0].implicitHeight : 0;
  }

  anchors.fill: parent

  Rectangle {
    anchors.fill: parent
    color: Theme.menu.scrim

    MouseArea {
      anchors.fill: parent
      onClicked: root.outsideClicked()
    }
  }

  Ui.BorderSurface {
    id: card

    objectName: root.cardName
    width: Math.min(root.width - Theme.spacing.xxl * 2, root.cardWidth)
    height: Math.min(root.height - Theme.spacing.xxl * 2,
      root.cardHeight > 0 ? root.cardHeight : root._contentHeight() + root.verticalInsets)
    color: Theme.popups.background
    borderSpec: Border.flat(Theme.popups.border, Style.normalBorderWidth)
    radius: Style.cornerRadius
    padding: Theme.spacing.panelPadding
    x: (root.width - card.width) / 2
    y: root.atTop ? Theme.spacing.xxl * 2 : (root.height - card.height) / 2

    // Clicks on the card stay on the card.
    MouseArea {
      anchors.fill: parent
    }

    Item {
      id: inner

      anchors {
        fill: parent
        topMargin: card.contentTopInset
        rightMargin: card.contentRightInset
        bottomMargin: card.contentBottomInset
        leftMargin: card.contentLeftInset
      }
    }
  }
}
