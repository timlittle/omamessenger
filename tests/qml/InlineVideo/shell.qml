// Checks InlineVideo's rendering and click intent, given fixed input
// properties, the way every other message media view is checked: a
// plain video asks to be opened externally while in-window playback is
// unavailable, but asks to toggle instead once it is; an animated "GIF"
// asks for its file as soon as it is shown, the same way a photo asks
// for its full image; a selected, playing video shows its controls row,
// and its own pause button flips without deselecting it (see
// _togglePause in InlineVideo.qml); a video whose fetch is known to
// have failed, with no thumbnail to fall back to, says so instead of
// staying an empty box; and a GIF or a selected, playing video asks to
// open the full-size viewer when its own frame is clicked, while a
// video not yet selected still just starts playing inline from that
// same click.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property int unavailableOpenedCount: 0
  property int availableToggledCount: 0
  property int animatedWantedCount: 0
  property int animatedExpandCount: 0
  property int playingExpandCount: 0

  FloatingWindow {
    implicitWidth: 500
    implicitHeight: 700
    visible: true

    Column {
      spacing: 4

      InlineVideo {
        id: unavailableVideo
        video: ({ kind: "video", width: 16, height: 9, duration: 12, thumb: "" })
        available: false
        onOpened: root.unavailableOpenedCount++
      }

      InlineVideo {
        id: availableVideo
        video: ({ kind: "video", width: 16, height: 9, duration: 12, thumb: "" })
        available: true
        path: "/does/not/matter.mp4"
        onToggled: root.availableToggledCount++
      }

      InlineVideo {
        id: animatedVideo
        video: ({ kind: "video", width: 16, height: 9, animated: true, thumb: "" })
        available: true
        onWanted: root.animatedWantedCount++
        onExpandRequested: root.animatedExpandCount++
      }

      InlineVideo {
        id: playingVideo
        video: ({ kind: "video", width: 16, height: 9, duration: 12, thumb: "" })
        available: true
        path: Qt.resolvedUrl(".") // a real, playable file is not needed for this check
          .toString().replace("file://", "") + "/does-not-exist.mp4"
        playing: true
        onExpandRequested: root.playingExpandCount++
      }

      InlineVideo {
        id: unreachableVideo
        video: ({ kind: "video", width: 16, height: 9, thumb: "" })
        failed: true
      }

      // A real GIF image (animated, with a genuine ".gif" name) must
      // animate through AnimatedImage even though in-window playback
      // (QtMultimedia) is available: the downloaded bytes are actual
      // GIF data, which only an image engine can decode, never a video
      // one. This is the bug the fix covers - before it, a real .gif
      // file silently loaded the video runtime instead and showed
      // nothing at all whenever QtMultimedia was present.
      InlineVideo {
        id: realGifVideo
        video: ({ kind: "video", width: 16, height: 9, animated: true, fileName: "run.gif", thumb: "" })
        available: true
        path: Qt.resolvedUrl(".").toString().replace("file://", "") + "/does-not-exist.gif"
      }
    }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    running: true
    interval: 50
    onTriggered: root.run()
  }

  function run(): void {
    // A video with no in-window playback available falls back to the
    // same "open externally" click a file or an unplayable voice note
    // already takes.
    t.mouseClick(unavailableVideo);
    if (root.unavailableOpenedCount !== 1) return Check.fail("clicking an unavailable video opened " + root.unavailableOpenedCount + " times, want 1");
    if (root.availableToggledCount !== 0) return Check.fail("clicking the unavailable video also toggled the available one");

    // Once in-window playback is available, the same click selects the
    // video instead of opening it externally, and does not also ask to
    // open the full-size viewer: a first click only ever starts inline
    // playback.
    t.mouseClick(availableVideo);
    if (root.availableToggledCount !== 1) return Check.fail("clicking an available video toggled " + root.availableToggledCount + " times, want 1");

    // An animated "GIF" asks for its file as soon as it is shown,
    // exactly like a photo's own thumbnail-to-full-image fetch.
    if (root.animatedWantedCount < 1) return Check.fail("an animated video with no path did not ask to be downloaded");

    // A GIF has no other click handler at all, so its whole frame asks
    // to open the full-size viewer.
    t.mouseClick(animatedVideo);
    if (root.animatedExpandCount !== 1) return Check.fail("clicking an animated GIF asked to expand " + root.animatedExpandCount + " times, want 1");

    const gifBadge = Check.find(animatedVideo, "gifBadge");
    if (!gifBadge || !gifBadge.visible) return Check.fail("an animated video shows no GIF badge");

    const unavailableBadge = Check.find(unavailableVideo, "gifBadge");
    if (unavailableBadge && unavailableBadge.visible) return Check.fail("an ordinary video shows a GIF badge");

    // A selected, playing video shows its controls, and its own pause
    // button stops and resumes playback without deselecting it.
    const controls = Check.find(playingVideo, "videoControls");
    if (!controls || !controls.visible) return Check.fail("a selected, playing video shows no controls row");

    const pauseButton = Check.find(playingVideo, "videoPauseButton");
    if (pauseButton.text !== "⏸") return Check.fail("a playing video's button is " + pauseButton.text + ", want the pause glyph");

    t.mouseClick(pauseButton);
    if (pauseButton.text !== "▶") return Check.fail("clicking pause did not switch to the play glyph");
    if (!playingVideo.playing) return Check.fail("pausing a video deselected it instead of just pausing it");

    // Once selected and playing, clicking the video's own frame (well
    // above its controls row, at the very top) asks to open the
    // full-size viewer instead of toggling it again.
    t.mouseClick(playingVideo, playingVideo.width / 2, 5);
    if (root.playingExpandCount !== 1) return Check.fail("clicking a playing video's frame asked to expand " + root.playingExpandCount + " times, want 1");

    // A video with no thumbnail whose fetch is known to have failed
    // says so instead of staying an empty box.
    const unavailableLabel = Check.find(unreachableVideo, "unavailableLabel");
    if (!unavailableLabel || !unavailableLabel.visible) return Check.fail("a failed video with no thumbnail shows no \"Video unavailable\" message");

    // A real .gif file animates through AnimatedImage, never the
    // QtMultimedia video runtime, even with in-window playback
    // available.
    const realGifImage = Check.find(realGifVideo, "realGifImage");
    if (!realGifImage || !realGifImage.visible) return Check.fail("a real .gif file with in-window playback available shows no AnimatedImage");

    const realGifThumb = Check.find(realGifVideo, "videoThumb");
    if (realGifThumb && realGifThumb.visible) return Check.fail("a real .gif file still shows its static thumbnail once its file is here");

    const realGifLoader = Check.find(realGifVideo, "videoRuntimeLoader");
    if (realGifLoader && realGifLoader.active) return Check.fail("a real .gif file loaded the QtMultimedia video runtime instead of playing through AnimatedImage");

    console.log("PASS InlineVideo");
    Qt.exit(0);
  }
}
