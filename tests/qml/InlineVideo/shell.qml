// Checks InlineVideo's rendering and click intent, given fixed input
// properties, the way every other message media view is checked: a
// plain video asks to be opened externally while in-window playback is
// unavailable, but asks to toggle instead once it is; an animated "GIF"
// asks for its file as soon as it is shown, the same way a photo asks
// for its full image; a selected, playing video shows its controls row,
// and its own pause button flips without deselecting it (see
// _togglePause in InlineVideo.qml); and a video whose fetch is known to
// have failed, with no thumbnail to fall back to, says so instead of
// staying an empty box.
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
      }

      InlineVideo {
        id: playingVideo
        video: ({ kind: "video", width: 16, height: 9, duration: 12, thumb: "" })
        available: true
        path: Qt.resolvedUrl(".") // a real, playable file is not needed for this check
          .toString().replace("file://", "") + "/does-not-exist.mp4"
        playing: true
      }

      InlineVideo {
        id: unreachableVideo
        video: ({ kind: "video", width: 16, height: 9, thumb: "" })
        failed: true
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
    // video instead of opening it externally.
    t.mouseClick(availableVideo);
    if (root.availableToggledCount !== 1) return Check.fail("clicking an available video toggled " + root.availableToggledCount + " times, want 1");

    // An animated "GIF" asks for its file as soon as it is shown,
    // exactly like a photo's own thumbnail-to-full-image fetch.
    if (root.animatedWantedCount < 1) return Check.fail("an animated video with no path did not ask to be downloaded");

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

    // A video with no thumbnail whose fetch is known to have failed
    // says so instead of staying an empty box.
    const unavailableLabel = Check.find(unreachableVideo, "unavailableLabel");
    if (!unavailableLabel || !unavailableLabel.visible) return Check.fail("a failed video with no thumbnail shows no \"Video unavailable\" message");

    console.log("PASS InlineVideo");
    Qt.exit(0);
  }
}
