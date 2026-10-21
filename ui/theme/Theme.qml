pragma Singleton
// qmllint disable missing-property
//
// Typed view of Omarchy's nested theme tokens, and the only file in this
// UI allowed to name Omarchy's Color singleton. Qt 6.12 added a global
// QML value type also named Color, which shadows any unqualified "Color"
// identifier once it is in scope; qualifying the import ("as Commons")
// and always writing Commons.Color keeps this file resolving Omarchy's
// singleton on both Qt 6.11 and 6.12 (see docs/decisions.md). Every other
// file in ui/ reads colours as Theme.foreground, Theme.accent, and so on,
// never Color.* directly, and tests/unit/no-bare-color.test.cjs enforces
// that. Omarchy also declares nested groups such as Style.font and
// Color.popups as plain QtObject properties, so qmllint cannot see their
// members and warns on every use; this adapter re-exports them as typed
// inline components with the same names, so the UI writes Theme.font.body
// or Theme.popups.background and qmllint still catches typos. It is the
// only file allowed to suppress missing-property. Plain tokens that are
// not colours (Style.space(), Util.alpha(), Style.hoverFill, ...) are
// still used from qs.Commons directly, unaffected by the Qt 6.12 clash.
//
// Generated from /usr/share/omarchy/shell/Commons (Omarchy 4.0.4); add a
// member here when the UI needs one Omarchy defines.
import QtQuick
import qs.Commons as Commons

QtObject {
  component Fonts: QtObject {
    readonly property string family: Commons.Style.font.family
    readonly property string resolvedFamily: Commons.Style.font.resolvedFamily
    readonly property string menuFamily: Commons.Style.font.menuFamily
    readonly property int baseSize: Commons.Style.font.baseSize
    readonly property int caption: Commons.Style.font.caption
    readonly property int bodySmall: Commons.Style.font.bodySmall
    readonly property int body: Commons.Style.font.body
    readonly property int subtitle: Commons.Style.font.subtitle
    readonly property int title: Commons.Style.font.title
    readonly property int heading: Commons.Style.font.heading
    readonly property int display: Commons.Style.font.display
    readonly property int displayLarge: Commons.Style.font.displayLarge
    readonly property int iconSmall: Commons.Style.font.iconSmall
    readonly property int icon: Commons.Style.font.icon
    readonly property int iconLarge: Commons.Style.font.iconLarge
  }

  component Spacing: QtObject {
    readonly property real scale: Commons.Style.spacing.scale
    readonly property int hairline: Commons.Style.spacing.hairline
    readonly property int xxs: Commons.Style.spacing.xxs
    readonly property int xs: Commons.Style.spacing.xs
    readonly property int sm: Commons.Style.spacing.sm
    readonly property int md: Commons.Style.spacing.md
    readonly property int lg: Commons.Style.spacing.lg
    readonly property int xl: Commons.Style.spacing.xl
    readonly property int xxl: Commons.Style.spacing.xxl
    readonly property int xxxl: Commons.Style.spacing.xxxl
    readonly property int huge: Commons.Style.spacing.huge
    readonly property int controlGap: Commons.Style.spacing.controlGap
    readonly property int controlPaddingX: Commons.Style.spacing.controlPaddingX
    readonly property int controlPaddingY: Commons.Style.spacing.controlPaddingY
    readonly property int inputPaddingY: Commons.Style.spacing.inputPaddingY
    readonly property int controlHeight: Commons.Style.spacing.controlHeight
    readonly property int popupRowHeight: Commons.Style.spacing.popupRowHeight
    readonly property int dropdownWidth: Commons.Style.spacing.dropdownWidth
    readonly property int searchableDropdownWidth: Commons.Style.spacing.searchableDropdownWidth
    readonly property int numberFieldWidth: Commons.Style.spacing.numberFieldWidth
    readonly property int searchablePopupMinHeight: Commons.Style.spacing.searchablePopupMinHeight
    readonly property int rowGap: Commons.Style.spacing.rowGap
    readonly property int rowPaddingX: Commons.Style.spacing.rowPaddingX
    readonly property int labelGap: Commons.Style.spacing.labelGap
    readonly property int panelGap: Commons.Style.spacing.panelGap
    readonly property int panelPadding: Commons.Style.spacing.panelPadding
    readonly property int popupPadding: Commons.Style.spacing.popupPadding
  }

  component BarSizes: QtObject {
    readonly property int sizeHorizontal: Commons.Style.bar.sizeHorizontal
    readonly property int sizeVertical: Commons.Style.bar.sizeVertical
    readonly property int iconSlot: Commons.Style.bar.iconSlot
    readonly property int iconCanvas: Commons.Style.bar.iconCanvas
    readonly property int iconFont: Commons.Style.bar.iconFont
    readonly property int statusSlot: Commons.Style.bar.statusSlot
  }

  component BarColors: QtObject {
    readonly property color background: Commons.Color.bar.background
    readonly property color text: Commons.Color.bar.text
    readonly property color active: Commons.Color.bar.active
  }

  component Popups: QtObject {
    readonly property color background: Commons.Color.popups.background
    readonly property color text: Commons.Color.popups.text
    readonly property color border: Commons.Color.popups.border
  }

  component Tooltip: QtObject {
    readonly property color background: Commons.Color.tooltip.background
    readonly property color text: Commons.Color.tooltip.text
    readonly property color border: Commons.Color.tooltip.border
  }

  component Menu: QtObject {
    readonly property color background: Commons.Color.menu.background
    readonly property color text: Commons.Color.menu.text
    readonly property color border: Commons.Color.menu.border
    readonly property color scrim: Commons.Color.menu.scrim
    readonly property color selectedBackground: Commons.Color.menu.selectedBackground
    readonly property color selectedText: Commons.Color.menu.selectedText
    readonly property color selectedBorder: Commons.Color.menu.selectedBorder
  }

  // foreground is Omarchy's primary text and icon colour.
  readonly property color foreground: Commons.Color.foreground

  // background is Omarchy's primary surface colour.
  readonly property color background: Commons.Color.background

  // accent is Omarchy's theme accent, used for the outgoing bubble,
  // selection fills and anything else that should read as "active".
  readonly property color accent: Commons.Color.accent

  // muted is Omarchy's de-emphasised text colour, for secondary labels.
  readonly property color muted: Commons.Color.muted

  // urgent is Omarchy's attention colour, for errors and unread badges.
  readonly property color urgent: Commons.Color.urgent

  // senderPaletteSize is how many distinct colours senderColor() cycles
  // through before a group chat's sender colours repeat.
  readonly property int senderPaletteSize: 8

  // senderColor returns one of senderPaletteSize colours for naming a
  // group chat's senders, derived by rotating the theme's own accent hue:
  // every index lands on a different hue than the accent itself, so a
  // sender's name is never mistaken for the outgoing-bubble colour, and
  // lightness is nudged away from the background when the accent would
  // otherwise read too close to it to stay legible.
  function senderColor(index: int): color {
    const accent = Commons.Color.accent
    const steps = ((index % senderPaletteSize) + senderPaletteSize) % senderPaletteSize
    const hueStep = 1 / (senderPaletteSize + 1)
    const hue = (accent.hslHue + hueStep * (steps + 1)) % 1

    const saturation = Math.max(accent.hslSaturation, 0.35)
    const bgLightness = Commons.Color.background.hslLightness
    let lightness = accent.hslLightness
    if (Math.abs(lightness - bgLightness) < 0.18) {
      lightness = bgLightness < 0.5 ? Math.min(0.78, bgLightness + 0.32) : Math.max(0.22, bgLightness - 0.32)
    }
    lightness = Math.max(0.3, Math.min(0.78, lightness))

    return Qt.hsla(hue, saturation, lightness, 1)
  }

  // accountPaletteSize is how many distinct colours accountColor() cycles
  // through before accounts' colour tags repeat.
  readonly property int accountPaletteSize: 6

  // accountColor returns one of accountPaletteSize colours for tagging an
  // account in the conversation list and the rail. It rotates the same
  // accent hue senderColor does, but out of phase with it and at a lower
  // saturation, so an account's colour tag never reads as one of a
  // group's sender colours on the rare row where both would show: the
  // hues fall between senderColor's own slots rather than on them, and
  // the muted saturation keeps the tag quieter than a sender name needs
  // to be.
  function accountColor(index: int): color {
    const accent = Commons.Color.accent
    const steps = ((index % accountPaletteSize) + accountPaletteSize) % accountPaletteSize
    const hueStep = 1 / (accountPaletteSize + 1)
    const hue = (accent.hslHue + hueStep * (steps + 1) + hueStep / 2) % 1

    const saturation = Math.max(accent.hslSaturation * 0.6, 0.2)
    const bgLightness = Commons.Color.background.hslLightness
    let lightness = accent.hslLightness
    if (Math.abs(lightness - bgLightness) < 0.18) {
      lightness = bgLightness < 0.5 ? Math.min(0.78, bgLightness + 0.32) : Math.max(0.22, bgLightness - 0.32)
    }
    lightness = Math.max(0.3, Math.min(0.78, lightness))

    return Qt.hsla(hue, saturation, lightness, 1)
  }

  readonly property Fonts font: Fonts {}
  readonly property Spacing spacing: Spacing {}
  readonly property BarSizes bar: BarSizes {}
  readonly property BarColors barColors: BarColors {}
  readonly property Popups popups: Popups {}
  readonly property Tooltip tooltip: Tooltip {}
  readonly property Menu menu: Menu {}
}
