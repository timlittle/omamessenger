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

  // realPngThumbBase64 is a tiny, real 8x8 PNG with a genuinely
  // transparent corner: imagesafe.Reencode writes a thumb this way
  // whenever hasAlpha finds a pixel that is not fully opaque, a
  // sticker's own cut-out shape above all (see docs/decisions.md), so
  // this proves Media.thumbMime's own sniff picks "image/png" and
  // that Qt actually decodes the result, not just that the Image
  // element exists.
  readonly property string realPngThumbBase64: "iVBORw0KGgoAAAANSUhEUgAAAAgAAAAIAgMAAAC5YVYYAAAAIGNIUk0AAHomAACAhAAA+gAAAIDoAAB1MAAA6mAAADqYAAAXcJy6UTwAAAAMUExURQAAAOBQUOBQUOBQUOREq+cAAAADdFJOUwAos/VDiEIAAAAHdElNRQfqCwIPGjrbrw+KAAAAJXRFWHRkYXRlOmNyZWF0ZQAyMDI2LTExLTAyVDE1OjI2OjU4KzAwOjAw5RV57wAAACV0RVh0ZGF0ZTptb2RpZnkAMjAyNi0xMS0wMlQxNToyNjo1OCswMDowMJRIwVMAAAAodEVYdGRhdGU6dGltZXN0YW1wADIwMjYtMTEtMDJUMTU6MjY6NTgrMDA6MDDDXeCMAAAAHUlEQVQI12NgYGBge8Ig/5dB/x+D/X8QCWSzPQEATWcHpT3VHv4AAAAASUVORK5CYII="

  function run(): void {
    if (!root.checkStickerWithNoThumbShowsEmoji()) return;
    if (!root.checkStickerWithThumbShowsImage()) return;
    if (!root.checkStickerHasNoBubbleFill()) return;
    if (!root.checkMentionsMeShowsGlyph()) return;
    if (!root.checkOversizedEmojiStaysBounded()) return;

    stepper.retry(root.checkRealPngThumbRenders);
  }

  // checkRealPngThumbRenders polls realPngSticker's own Image until it
  // leaves the loading state: Ready and a positive sourceSize means Qt
  // actually decoded the re-encoded PNG thumb, not just that the
  // element exists with its own visible binding true.
  function checkRealPngThumbRenders(): void {
    const image = Check.find(realPngSticker, "stickerImage");
    if (!image) { Check.fail("could not find realPngSticker's own Image"); return; }
    if (image.status === Image.Error) { Check.fail("real PNG sticker thumb failed to decode"); return; }
    if (image.status !== Image.Ready) { stepper.retry(root.checkRealPngThumbRenders); return; }
    if (image.sourceSize.width <= 0 || image.sourceSize.height <= 0) { Check.fail("real PNG sticker thumb decoded to an empty size"); return; }

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

  // checkOversizedEmojiStaysBounded covers StickerView's own defence in
  // depth: domain.CapMedia already bounds a sticker's emoji before it
  // ever reaches the UI, but a connector bug or an older helper could
  // still hand over something uncapped, so the fallback text must wrap
  // and elide rather than lay out as one unbounded line.
  function checkOversizedEmojiStaysBounded(): bool {
    const emoji = Check.find(stickerHugeEmoji, "stickerEmoji");
    if (!emoji || !emoji.visible) return Check.fail("stickerEmoji not shown for the oversized-emoji sticker");
    if (emoji.wrapMode !== Text.Wrap) return Check.fail("stickerEmoji wrapMode = " + emoji.wrapMode + ", want Text.Wrap");
    if (emoji.maximumLineCount !== 2) return Check.fail("stickerEmoji maximumLineCount = " + emoji.maximumLineCount + ", want 2");
    if (emoji.elide !== Text.ElideRight) return Check.fail("stickerEmoji elide = " + emoji.elide + ", want Text.ElideRight");
    if (emoji.contentWidth > emoji.width + 1) return Check.fail("stickerEmoji contentWidth " + emoji.contentWidth + " exceeds its bounded width " + emoji.width);
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
        id: stickerHugeEmoji
        width: 600
        annotation: root.noAnnotation
        message: ({
          id: "s3", outgoing: false, status: "received", created: 0, senderName: "", text: "[Sticker]",
          media: { kind: "sticker", emoji: "x".repeat(500) }
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

      MessageDelegate {
        id: realPngSticker
        width: 600
        annotation: root.noAnnotation
        message: ({
          id: "s4", outgoing: false, status: "received", created: 0, senderName: "", text: "[Sticker]",
          media: { kind: "sticker", emoji: "😀", thumb: root.realPngThumbBase64 }
        })
      }
    }
  }

  // Layout settles over a few frames, so measure after a short delay.
  Timer {
    running: true
    interval: 100
    onTriggered: root.run()
  }

  Stepper {
    id: stepper
    name: "MessageSticker"
  }
}
