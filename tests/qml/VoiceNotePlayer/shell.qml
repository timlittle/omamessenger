// Checks VoiceNotePlayer: an available player shows a play button, a
// progress bar and an elapsed/total time label and asks for its file as
// soon as it is shown; clicking the button reports "opened" either way;
// a note known to have failed with nothing downloaded shows "Unavailable"
// instead of offering to play nothing; and when the optional QtMultimedia
// player could not load, the whole thing falls back to the same plain
// "open externally" row FileView uses, with the same click behaviour.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var wanted: []
  property var opened: []

  function run(): void {
    if (!root.checkAvailablePlayer()) return;
    if (!root.checkPlayingGlyph()) return;
    if (!root.checkUnplayable()) return;
    if (!root.checkFallback()) return;

    console.log("PASS VoiceNotePlayer");
    Qt.exit(0);
  }

  // checkAvailablePlayer verifies an available player asks to be
  // downloaded as soon as it is shown, and shows its label, a progress
  // bar and the known duration before anything has played.
  function checkAvailablePlayer(): bool {
    if (root.wanted.indexOf("seen") < 0) return Check.fail("a voice note did not ask to be downloaded once shown");

    const label = Check.find(player, "voiceAccessibleLabel");
    if (!label || label.text !== "Voice message") return Check.fail("voice note has no \"Voice message\" label");

    const time = Check.find(player, "voiceTimeLabel");
    if (time.text !== "0:00 / 0:12") return Check.fail("time label is \"" + time.text + "\", want \"0:00 / 0:12\"");

    const fill = Check.find(player, "voiceProgressFill");
    if (fill.width !== 0) return Check.fail("progress bar is not empty before playback starts");

    const button = Check.find(player, "voicePlayButton");
    if (button.text !== "▶") return Check.fail("play button does not show the play glyph");

    t.mouseClick(button);
    if (root.opened.length !== 1) return Check.fail("clicking the play button did not report \"opened\"");
    return true;
  }

  // checkPlayingGlyph verifies the button and progress bar follow
  // playing, positionMs and durationMs once the caller reports them.
  function checkPlayingGlyph(): bool {
    player.playing = true;
    player.positionMs = 6000;
    player.durationMs = 12000;

    const button = Check.find(player, "voicePlayButton");
    if (button.text !== "⏸") return Check.fail("playing voice note still shows the play glyph");

    const time = Check.find(player, "voiceTimeLabel");
    if (time.text !== "0:06 / 0:12") return Check.fail("time label is \"" + time.text + "\" mid-playback, want \"0:06 / 0:12\"");

    const fill = Check.find(player, "voiceProgressFill");
    const track = fill.parent;
    if (Math.abs(fill.width - track.width / 2) > 1) return Check.fail("progress bar is not half full at the halfway point");

    player.playing = false;
    player.positionMs = 0;
    return true;
  }

  // checkUnplayable verifies a note with a known failed fetch and no
  // downloaded copy disables its button and says so, rather than
  // offering to play nothing.
  function checkUnplayable(): bool {
    player.failed = true;
    player.path = "";

    const button = Check.find(player, "voicePlayButton");
    if (button.enabled) return Check.fail("play button stays enabled for a note known to have failed");

    const time = Check.find(player, "voiceTimeLabel");
    if (time.text !== "Unavailable") return Check.fail("time label is \"" + time.text + "\", want \"Unavailable\"");

    player.failed = false;
    return true;
  }

  // checkFallback verifies that with no in-window player available, the
  // plain "open externally" row shows instead, and clicking it still
  // reports "opened" so the caller can download and open the file.
  function checkFallback(): bool {
    if (Check.find(unavailablePlayer, "voicePlayerRow").visible) return Check.fail("an unavailable player still shows its play button row");
    if (!Check.find(unavailablePlayer, "voiceFallbackRow").visible) return Check.fail("unavailable player does not show the plain \"open\" row");

    const label = root.findText(Check.texts(unavailablePlayer), "Voice message · open");
    if (!label) return Check.fail("unavailable player does not show \"Voice message · open\"");

    root.opened = [];
    t.mouseClick(unavailablePlayer);
    if (root.opened.length !== 1) return Check.fail("clicking the unavailable row did not report \"opened\"");
    return true;
  }

  // findText returns the first node whose text matches exactly.
  function findText(nodes, text) {
    return nodes.find((node) => node.text === text) ?? null;
  }

  FloatingWindow {
    implicitWidth: 400
    implicitHeight: 300
    visible: true

    Column {
      VoiceNotePlayer {
        id: player
        media: ({ kind: "voice", duration: 12, fileName: "voice-message.ogg" })
        available: true
        onWanted: root.wanted.push("seen")
        onOpened: root.opened.push("seen")
      }

      VoiceNotePlayer {
        id: unavailablePlayer
        media: ({ kind: "voice", duration: 12, fileName: "voice-message.ogg" })
        available: false
        onOpened: root.opened.push("seen")
      }
    }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }
}
