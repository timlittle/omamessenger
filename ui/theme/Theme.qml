pragma Singleton
// qmllint disable missing-property
//
// Typed view of Omarchy's nested theme tokens. Omarchy declares groups such as
// Style.font and Color.popups as plain QtObject properties, so qmllint cannot
// see their members and warns on every use (F15). This adapter re-exports them
// as typed inline components with the same names, so the UI writes
// Theme.font.body or Theme.popups.background and qmllint still catches typos.
// It is the only file allowed to suppress missing-property. Plain tokens
// (Color.foreground, Style.space(), Util.alpha(), Style.hoverFill, ...) are
// used from qs.Commons directly.
//
// Generated from /usr/share/omarchy/shell/Commons (Omarchy 4.0.4); add a
// member here when the UI needs one Omarchy defines.
import QtQuick
import qs.Commons

QtObject {
    component Fonts: QtObject {
        readonly property string family: Style.font.family
        readonly property string resolvedFamily: Style.font.resolvedFamily
        readonly property string menuFamily: Style.font.menuFamily
        readonly property int baseSize: Style.font.baseSize
        readonly property int caption: Style.font.caption
        readonly property int bodySmall: Style.font.bodySmall
        readonly property int body: Style.font.body
        readonly property int subtitle: Style.font.subtitle
        readonly property int title: Style.font.title
        readonly property int heading: Style.font.heading
        readonly property int display: Style.font.display
        readonly property int displayLarge: Style.font.displayLarge
        readonly property int iconSmall: Style.font.iconSmall
        readonly property int icon: Style.font.icon
        readonly property int iconLarge: Style.font.iconLarge
    }

    component Spacing: QtObject {
        readonly property real scale: Style.spacing.scale
        readonly property int hairline: Style.spacing.hairline
        readonly property int xxs: Style.spacing.xxs
        readonly property int xs: Style.spacing.xs
        readonly property int sm: Style.spacing.sm
        readonly property int md: Style.spacing.md
        readonly property int lg: Style.spacing.lg
        readonly property int xl: Style.spacing.xl
        readonly property int xxl: Style.spacing.xxl
        readonly property int xxxl: Style.spacing.xxxl
        readonly property int huge: Style.spacing.huge
        readonly property int controlGap: Style.spacing.controlGap
        readonly property int controlPaddingX: Style.spacing.controlPaddingX
        readonly property int controlPaddingY: Style.spacing.controlPaddingY
        readonly property int inputPaddingY: Style.spacing.inputPaddingY
        readonly property int controlHeight: Style.spacing.controlHeight
        readonly property int popupRowHeight: Style.spacing.popupRowHeight
        readonly property int dropdownWidth: Style.spacing.dropdownWidth
        readonly property int searchableDropdownWidth: Style.spacing.searchableDropdownWidth
        readonly property int numberFieldWidth: Style.spacing.numberFieldWidth
        readonly property int searchablePopupMinHeight: Style.spacing.searchablePopupMinHeight
        readonly property int rowGap: Style.spacing.rowGap
        readonly property int rowPaddingX: Style.spacing.rowPaddingX
        readonly property int labelGap: Style.spacing.labelGap
        readonly property int panelGap: Style.spacing.panelGap
        readonly property int panelPadding: Style.spacing.panelPadding
        readonly property int popupPadding: Style.spacing.popupPadding
    }

    component BarSizes: QtObject {
        readonly property int sizeHorizontal: Style.bar.sizeHorizontal
        readonly property int sizeVertical: Style.bar.sizeVertical
        readonly property int iconSlot: Style.bar.iconSlot
        readonly property int iconCanvas: Style.bar.iconCanvas
        readonly property int iconFont: Style.bar.iconFont
        readonly property int statusSlot: Style.bar.statusSlot
    }

    component BarColors: QtObject {
        readonly property color background: Color.bar.background
        readonly property color text: Color.bar.text
        readonly property color active: Color.bar.active
    }

    component Popups: QtObject {
        readonly property color background: Color.popups.background
        readonly property color text: Color.popups.text
        readonly property color border: Color.popups.border
    }

    component Tooltip: QtObject {
        readonly property color background: Color.tooltip.background
        readonly property color text: Color.tooltip.text
        readonly property color border: Color.tooltip.border
    }

    component Menu: QtObject {
        readonly property color background: Color.menu.background
        readonly property color text: Color.menu.text
        readonly property color border: Color.menu.border
        readonly property color scrim: Color.menu.scrim
        readonly property color selectedBackground: Color.menu.selectedBackground
        readonly property color selectedText: Color.menu.selectedText
        readonly property color selectedBorder: Color.menu.selectedBorder
    }

    readonly property Fonts font: Fonts {}
    readonly property Spacing spacing: Spacing {}
    readonly property BarSizes bar: BarSizes {}
    readonly property BarColors barColors: BarColors {}
    readonly property Popups popups: Popups {}
    readonly property Tooltip tooltip: Tooltip {}
    readonly property Menu menu: Menu {}
}
