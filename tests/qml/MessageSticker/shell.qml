// Checks a sticker message: it renders through StickerView with no
// bubble fill behind it, falls back to its emoji when there is no
// thumbnail or downloaded image yet, and shows its thumbnail once one
// arrives. Also checks the @-mention highlight: a message that
// mentions the signed-in user shows the paired "@" glyph and keeps a
// fainter outline even while not the keyboard's current highlight.
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  readonly property var noAnnotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })

  function run(): void {
    if (!root.checkStickerWithNoThumbShowsEmoji()) return;
    if (!root.checkStickerWithThumbShowsImage()) return;
    if (!root.checkStickerHasNoBubbleFill()) return;
    if (!root.checkMentionsMeShowsGlyph()) return;

    console.log("PASS MessageSticker");
    Qt.exit(0);
  }

  function checkStickerWithNoThumbShowsEmoji(): bool {
    const view = Check.find(stickerNoThumb, "stickerView");
    if (!view || !view.visible) return Check.fail("stickerView not shown for a sticker message");

    const emoji = Check.find(stickerNoThumb, "stickerEmoji");
    if (!emoji || !emoji.visible) return Check.fail("stickerEmoji not shown with no thumbnail or path");
    if (emoji.text !== "😀") return Check.fail("emoji text = " + emoji.text + ", want 😀");

    const image = Check.find(stickerNoThumb, "stickerImage");
    if (image && image.visible) return Check.fail("stickerImage shown with nothing to draw");
    return true;
  }

  function checkStickerWithThumbShowsImage(): bool {
    const image = Check.find(stickerWithThumb, "stickerImage");
    if (!image || !image.visible) return Check.fail("stickerImage not shown once a thumbnail exists");

    const emoji = Check.find(stickerWithThumb, "stickerEmoji");
    if (emoji && emoji.visible) return Check.fail("stickerEmoji shown even though a thumbnail exists");
    return true;
  }

  function checkStickerHasNoBubbleFill(): bool {
    const bubble = Check.find(stickerNoThumb, "bubble");
    if (!bubble) return Check.fail("no bubble found for the sticker message");
    if (String(bubble.color) !== "#00000000" && bubble.color.a !== 0)
      return Check.fail("sticker bubble.color = " + bubble.color + ", want fully transparent");
    return true;
  }

  function checkMentionsMeShowsGlyph(): bool {
    const glyph = Check.find(mentioned, "mentionGlyph");
    if (!glyph || !glyph.visible) return Check.fail("mentionGlyph not shown for a message that mentions me");

    const plainGlyph = Check.find(plain, "mentionGlyph");
    if (plainGlyph && plainGlyph.visible) return Check.fail("mentionGlyph shown for a message that does not mention me");

    const bubble = Check.find(mentioned, "bubble");
    if (bubble.border.width <= 0) return Check.fail("mentioned bubble has no outline while not highlighted");
    return true;
  }

  FloatingWindow {
    implicitWidth: 600
    implicitHeight: 500
    visible: true

    Column {
      width: 600

      MessageDelegate {
        id: stickerNoThumb
        width: 600
        annotation: root.noAnnotation
        message: ({
          id: "s1", outgoing: false, status: "received", created: 0, senderName: "", text: "[Sticker]",
          media: { kind: "sticker", emoji: "😀" }
        })
      }

      MessageDelegate {
        id: stickerWithThumb
        width: 600
        annotation: root.noAnnotation
        message: ({
          id: "s2", outgoing: false, status: "received", created: 0, senderName: "", text: "[Sticker]",
          media: { kind: "sticker", emoji: "😀", thumb: "Zm9v" }
        })
      }

      MessageDelegate {
        id: mentioned
        width: 600
        annotation: root.noAnnotation
        message: ({
          id: "m1", outgoing: false, status: "received", created: 0, senderName: "Nadia",
          text: "hi @You", mentionsMe: true
        })
      }

      MessageDelegate {
        id: plain
        width: 600
        annotation: root.noAnnotation
        message: ({ id: "m2", outgoing: false, status: "received", created: 0, senderName: "Nadia", text: "hi" })
      }
    }
  }

  // Layout settles over a few frames, so measure after a short delay.
  Timer {
    running: true
    interval: 100
    onTriggered: root.run()
  }
}
