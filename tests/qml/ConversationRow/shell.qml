// Checks ConversationRow's dimming: a plain row is drawn at full opacity
// with no label, a row dimmed only for being older is faded but carries
// no label (its timestamp already explains it), and a hidden or archived
// row is faded with a "Hidden" or "Archived" label, so the cue is never
// colour or opacity alone. Also checks the pin and mute markers: a pinned
// or muted row's glyph actually renders (non-zero width, not just an
// empty or unmapped-codepoint string), and a pinned row also carries a
// "Pinned" tag, so a chat sorted to the top of the list is not left with
// only a glyph as its only cue. Also checks the account colour stripe:
// hidden for a single-account row, shown and tinted with the row's own
// account colour otherwise, always paired with a tooltip naming the
// account.
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  ConversationRow {
    id: plainRow
    width: 260
    conversation: ({ id: "c1", title: "Plain", lastActivity: Date.now() })
    dimmed: false
    dimLabel: ""
  }

  ConversationRow {
    id: olderRow
    width: 260
    conversation: ({ id: "c2", title: "Older", lastActivity: 0 })
    dimmed: true
    dimLabel: ""
  }

  ConversationRow {
    id: hiddenRow
    width: 260
    conversation: ({ id: "c3", title: "Hidden Chat", lastActivity: Date.now() })
    dimmed: true
    dimLabel: "Hidden"
  }

  ConversationRow {
    id: archivedRow
    width: 260
    conversation: ({ id: "c4", title: "Archived Chat", lastActivity: Date.now() })
    dimmed: true
    dimLabel: "Archived"
  }

  ConversationRow {
    id: pinnedRow
    width: 260
    conversation: ({ id: "c5", title: "Pinned Chat", lastActivity: Date.now(), pinned: true })
  }

  ConversationRow {
    id: mutedRow
    width: 260
    conversation: ({ id: "c6", title: "Muted Chat", lastActivity: Date.now(), muted: true })
  }

  ConversationRow {
    id: taggedRow
    width: 260
    conversation: ({ id: "c7", title: "Tagged Chat", lastActivity: Date.now() })
    showAccountColor: true
    accountColor: "#ff00ff"
    accountName: "Work"
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run drives the rows and checks the outcome.
  function run(): void {
    if (plainRow.opacity !== 1)
      return Check.fail("a plain row is dimmed: opacity " + plainRow.opacity);
    const plainLabel = Check.find(plainRow, "dimLabel");
    if (!plainLabel || plainLabel.visible)
      return Check.fail("a plain row shows a dim label");

    if (olderRow.opacity >= 1)
      return Check.fail("an older row is not faded: opacity " + olderRow.opacity);
    const olderLabel = Check.find(olderRow, "dimLabel");
    if (!olderLabel || olderLabel.visible)
      return Check.fail("a merely older row shows a dim label, want none: its timestamp already explains it");

    if (hiddenRow.opacity >= 1)
      return Check.fail("a hidden row is not faded: opacity " + hiddenRow.opacity);
    const hiddenLabel = Check.find(hiddenRow, "dimLabel");
    if (!hiddenLabel || !hiddenLabel.visible || hiddenLabel.text !== "Hidden")
      return Check.fail("a hidden row does not carry a \"Hidden\" label");

    if (archivedRow.opacity >= 1)
      return Check.fail("an archived row is not faded: opacity " + archivedRow.opacity);
    const archivedLabel = Check.find(archivedRow, "dimLabel");
    if (!archivedLabel || !archivedLabel.visible || archivedLabel.text !== "Archived")
      return Check.fail("an archived row does not carry an \"Archived\" label");

    const plainPin = Check.find(plainRow, "pinIcon");
    if (!plainPin || plainPin.visible)
      return Check.fail("an unpinned row shows a pin icon");
    const plainPinnedLabel = Check.find(plainRow, "pinnedLabel");
    if (!plainPinnedLabel || plainPinnedLabel.visible)
      return Check.fail("an unpinned row shows a \"Pinned\" tag");
    const plainMute = Check.find(plainRow, "muteIcon");
    if (!plainMute || plainMute.visible)
      return Check.fail("an unmuted row shows a mute icon");

    const pinIcon = Check.find(pinnedRow, "pinIcon");
    if (!pinIcon || !pinIcon.visible)
      return Check.fail("a pinned row does not show a pin icon");
    if (pinIcon.implicitWidth <= 0)
      return Check.fail("a pinned row's pin glyph has no width: \"" + pinIcon.text + "\" does not render in this font");
    const pinnedLabel = Check.find(pinnedRow, "pinnedLabel");
    if (!pinnedLabel || !pinnedLabel.visible || pinnedLabel.text !== "Pinned")
      return Check.fail("a pinned row does not carry a \"Pinned\" tag beside its glyph");

    const muteIcon = Check.find(mutedRow, "muteIcon");
    if (!muteIcon || !muteIcon.visible)
      return Check.fail("a muted row does not show a mute icon");
    if (muteIcon.implicitWidth <= 0)
      return Check.fail("a muted row's mute glyph has no width: \"" + muteIcon.text + "\" does not render in this font");
    const mutedPinnedLabel = Check.find(mutedRow, "pinnedLabel");
    if (mutedPinnedLabel.visible)
      return Check.fail("a merely muted row shows a \"Pinned\" tag");

    const plainStripe = Check.find(plainRow, "accountColorStripe");
    if (!plainStripe || plainStripe.visible)
      return Check.fail("a single-account row shows an account colour stripe");

    const stripe = Check.find(taggedRow, "accountColorStripe");
    if (!stripe || !stripe.visible)
      return Check.fail("a multi-account row does not show its account colour stripe");
    if (String(stripe.color) !== "#ff00ff")
      return Check.fail("the account colour stripe is not tinted with its account colour: " + stripe.color);
    const tooltip = Check.find(taggedRow, "accountColorTooltip");
    if (!tooltip || tooltip.text !== "Work")
      return Check.fail("the account colour stripe has no tooltip naming its account, colour is not the only cue it must avoid being");

    console.log("PASS ConversationRow");
    Qt.exit(0);
  }
}
