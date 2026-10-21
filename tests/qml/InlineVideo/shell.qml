// Checks InlineVideo's rendering and click intent, given fixed input
// properties, the way every other message media view is checked: a
// plain video asks to be opened externally while in-window playback is
// unavailable, but asks to toggle instead once it is; an animated "GIF"
// shows a thumbnail and a play mark exactly like any other video, and
// never fetches or decodes without a click; a selected, playing video
// shows its controls row, and its own pause button flips without
// deselecting it (see _togglePause in InlineVideo.qml); a video whose
// fetch is known to have failed, with no thumbnail to fall back to,
// says so instead of staying an empty box; a selected, playing video
// (GIF or ordinary) asks to open the full-size viewer when its own
// frame is clicked, while one not yet selected still just starts
// playing inline from that same click; a video the helper has not
// validated as playable never decodes even once selected; and, of the
// single decoding slot every video shares, only the selected one ever
// decodes at once.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property int unavailableOpenedCount: 0
  property int availableToggledCount: 0
  property int animatedToggledCount: 0
  property int animatedExpandCount: 0
  property int playingExpandCount: 0
  property int realGifExpandCount: 0

  // selectedId stands in for the shared playingId a real
  // VideoController tracks: sharedA and sharedB below both bind their
  // own playing property to it, the same way MessageBubbleContent
  // binds every bubble's InlineVideo to one controller's playingId.
  property string selectedId: ""

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
        onToggled: root.animatedToggledCount++
        onExpandRequested: root.animatedExpandCount++
      }

      InlineVideo {
        id: playingVideo
        video: ({ kind: "video", width: 16, height: 9, duration: 12, thumb: "" })
        available: true
        playable: true
        path: Qt.resolvedUrl(".") // a real, playable file is not needed for this check
          .toString().replace("file://", "") + "/does-not-exist.mp4"
        playing: true
        onExpandRequested: root.playingExpandCount++
      }

      InlineVideo {
        id: unplayableVideo
        video: ({ kind: "video", width: 16, height: 9, duration: 12, thumb: "" })
        available: true
        playable: false
        path: Qt.resolvedUrl(".").toString().replace("file://", "") + "/does-not-exist-unplayable.mp4"
        playing: true
      }

      InlineVideo {
        id: unreachableVideo
        video: ({ kind: "video", width: 16, height: 9, thumb: "" })
        failed: true
      }

      // A real GIF image (confirmed by the helper's own decodeAs
      // verdict, never the fileName or animated flag alone) must
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
        decodeAs: "gif"
        playing: true
        path: Qt.resolvedUrl(".").toString().replace("file://", "") + "/does-not-exist.gif"
        onExpandRequested: root.realGifExpandCount++
      }

      // A video merely named and flagged like a GIF, but with no
      // decodeAs verdict from the helper, must never animate through
      // AnimatedImage: the exact exploit this fix closes, where a
      // sender set fileName and the animated attribute on a video the
      // helper had not sanitized or confirmed as GIF data at all.
      InlineVideo {
        id: fakeGifVideo
        video: ({ kind: "video", width: 16, height: 9, animated: true, fileName: "evil.gif", thumb: "" })
        available: true
        decodeAs: ""
        path: Qt.resolvedUrl(".").toString().replace("file://", "") + "/does-not-exist-either.mp4"
      }

      InlineVideo {
        id: sharedA
        video: ({ kind: "video", width: 16, height: 9, duration: 12, thumb: "" })
        available: true
        playable: true
        path: Qt.resolvedUrl(".").toString().replace("file://", "") + "/does-not-exist-a.mp4"
        playing: root.selectedId === "a"
      }

      InlineVideo {
        id: sharedB
        video: ({ kind: "video", width: 16, height: 9, duration: 12, thumb: "" })
        available: true
        playable: true
        path: Qt.resolvedUrl(".").toString().replace("file://", "") + "/does-not-exist-b.mp4"
        playing: root.selectedId === "b"
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

    // An animated "GIF" never fetches or decodes on its own: it shows
    // a thumbnail and a play mark exactly like any other video, and a
    // click is what asks the caller to fetch, validate and select it,
    // the same toggled() every other video's first click fires.
    const animatedPlayMark = Check.find(animatedVideo, "playMark");
    if (!animatedPlayMark || !animatedPlayMark.visible) return Check.fail("an animated video with no path shows no play mark");
    if (root.animatedToggledCount !== 0) return Check.fail("an animated video asked to toggle before any click");

    t.mouseClick(animatedVideo);
    if (root.animatedToggledCount !== 1) return Check.fail("clicking an animated video with no path toggled " + root.animatedToggledCount + " times, want 1");
    if (root.animatedExpandCount !== 0) return Check.fail("clicking an animated video not yet selected asked to expand instead of toggling");

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

    // A video selected for playback but never validated as playable
    // never loads the QtMultimedia runtime at all: the helper's own
    // verdict, not just selection, decides whether anything decodes.
    const unplayableLoader = Check.find(unplayableVideo, "videoRuntimeLoader");
    if (unplayableLoader && unplayableLoader.active) return Check.fail("a selected but unvalidated video loaded the video runtime anyway");

    // A video with no thumbnail whose fetch is known to have failed
    // says so instead of staying an empty box.
    const unavailableLabel = Check.find(unreachableVideo, "unavailableLabel");
    if (!unavailableLabel || !unavailableLabel.visible) return Check.fail("a failed video with no thumbnail shows no \"Video unavailable\" message");

    // A real .gif file animates through AnimatedImage, never the
    // QtMultimedia video runtime, even with in-window playback
    // available, and shows no play mark once it is already animating.
    const realGifImage = Check.find(realGifVideo, "realGifImage");
    if (!realGifImage || !realGifImage.visible) return Check.fail("a real .gif file with in-window playback available shows no AnimatedImage");

    const realGifThumb = Check.find(realGifVideo, "videoThumb");
    if (realGifThumb && realGifThumb.visible) return Check.fail("a real .gif file still shows its static thumbnail once its file is here");

    const realGifLoader = Check.find(realGifVideo, "videoRuntimeLoader");
    if (realGifLoader && realGifLoader.active) return Check.fail("a real .gif file loaded the QtMultimedia video runtime instead of playing through AnimatedImage");

    const realGifPlayMark = Check.find(realGifVideo, "playMark");
    if (realGifPlayMark && realGifPlayMark.visible) return Check.fail("a real .gif file already animating still shows a play mark");

    // Once selected, a real GIF's whole frame still asks to open the
    // full-size viewer, the same as any other selected, playing video.
    t.mouseClick(realGifVideo);
    if (root.realGifExpandCount !== 1) return Check.fail("clicking a selected, playing real GIF toggled it instead of asking to expand");

    // A video merely named and flagged like a GIF, with no decodeAs
    // verdict, never animates through AnimatedImage: it stays an
    // ordinary video, exactly like unavailableVideo above.
    const fakeGifImage = Check.find(fakeGifVideo, "realGifImage");
    if (fakeGifImage && fakeGifImage.visible) return Check.fail("a fileName and animated flag alone picked AnimatedImage with no decodeAs verdict");

    // Of the single decoding slot every video shares, only the one
    // the shared selection actually names ever decodes: selecting "a"
    // never leaves "b" decoding too, and switching to "b" tears "a"'s
    // engine down rather than running both side by side.
    root.selectedId = "a";
    let sharedALoader = Check.find(sharedA, "videoRuntimeLoader");
    let sharedBLoader = Check.find(sharedB, "videoRuntimeLoader");
    if (!sharedALoader.active) return Check.fail("selecting \"a\" did not start its own decoder");
    if (sharedBLoader.active) return Check.fail("selecting \"a\" also left \"b\" decoding");

    root.selectedId = "b";
    if (sharedALoader.active) return Check.fail("selecting \"b\" left \"a\" still decoding: two bubbles decoded at once");
    if (!sharedBLoader.active) return Check.fail("selecting \"b\" did not start its own decoder");

    console.log("PASS InlineVideo");
    Qt.exit(0);
  }
}
