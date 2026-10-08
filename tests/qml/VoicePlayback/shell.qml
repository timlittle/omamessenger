// Drives the real Panel, Service and test helper end to end for a voice
// note: Ctrl+K to the dentist's chat, whose newest seeded message is a
// voice note, Escape to leave the composer, Enter to open it (the same
// path the play button's click takes), and then waits for the real
// helper to download it and the in-window player to start playing a
// real file. This exercises the whole seam a fake-controller unit test
// cannot reach: the RPC round trip, app.Commands.FetchMedia, the fake
// connector's own download and the path landing back on the message, so
// a bug in any of those shows up here even when each piece's own test
// still passes. XDG_DATA_HOME is set by the test runner, so this never
// touches real data.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  Service {
    id: service
  }

  FakeShell {
    id: fakeShell
    panel: panel
    service: service
  }

  Panel {
    id: panel
    service: service
    shell: fakeShell
  }

  TestCase {
    id: t
    when: false
  }

  Stepper {
    id: stepper
    name: "VoicePlayback"
    deadlineMs: 55000
    onTimeout: () => Check.fail("timed out before the checks finished")
    startFn: root.start
  }

  // start opens the window and waits for the fake accounts to finish
  // seeding, the same race every other test against the test helper has
  // to account for.
  function start(): void {
    panel.open("{}");
    root.waitForConversations();
  }

  function waitForConversations(): void {
    const listView = Check.find(panel, "conversationListView");
    if (listView && listView.count === 11) return root.openDentist();

    stepper.attempts++;
    if (stepper.attempts >= 200)
      return Check.fail("got " + (listView ? listView.count : "no list view") + " conversations after retrying, want 11");
    stepper.retry(root.waitForConversations);
  }

  // openDentist jumps to the dentist's chat with Ctrl+K, the conversation
  // switcher, typing enough of the title to match it uniquely.
  function openDentist(): void {
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    for (const ch of "dentist") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);

    stepper.attempts = 0;
    root.waitForDentistOpen();
  }

  // waitForDentistOpen holds until the dentist's chat is the open
  // conversation and its two seeded messages are loaded.
  function waitForDentistOpen(): void {
    const title = Check.find(panel, "conversationTitle");
    const messages = Check.find(panel, "messageListView");

    if (title && title.text.indexOf("Dentist") !== -1 && messages && messages.model && messages.model.count === 2)
      return root.leaveComposer();

    stepper.attempts++;
    if (stepper.attempts >= 200)
      return Check.fail("the dentist's chat never opened with its two messages: title=\""
        + (title ? title.text : "?") + "\"");
    stepper.retry(root.waitForDentistOpen);
  }

  // leaveComposer presses Escape to blur the composer, so the Enter that
  // follows opens the highlighted message instead of typing into it or
  // sending an empty one.
  function leaveComposer(): void {
    t.keyClick(Qt.Key_Escape);

    stepper.attempts = 0;
    root.waitForComposerLeft();
  }

  function waitForComposerLeft(): void {
    const composer = Check.find(panel, "composerInput");
    if (composer && !composer.activeFocus) return root.openHighlightedVoiceNote();

    stepper.attempts++;
    if (stepper.attempts >= 200) return Check.fail("Escape never left the composer");
    stepper.retry(root.waitForComposerLeft);
  }

  // openHighlightedVoiceNote waits for the newest message's voice note
  // player to actually exist in the list, which lags slightly behind
  // the model filling once a chat opens (its delegate is still being
  // instantiated), then presses Enter on it: the same action the play
  // button's own click takes.
  function openHighlightedVoiceNote(): void {
    const player = Check.find(panel, "voiceNotePlayer");
    if (player && player.visible) {
      t.keyClick(Qt.Key_Return);

      stepper.attempts = 0;
      root.waitForPlaying();
      return;
    }

    stepper.attempts++;
    if (stepper.attempts >= 200)
      return Check.fail("the newest message in the dentist's chat never showed a voice note player");
    stepper.retry(root.openHighlightedVoiceNote);
  }

  // waitForPlaying holds until the helper has downloaded the voice note
  // and the in-window player reports a real path and started playing,
  // never landing on the "Unavailable" state a silent fetch failure (the
  // bug this test guards against) would leave it in.
  function waitForPlaying(): void {
    const player = Check.find(panel, "voiceNotePlayer");
    const button = Check.find(panel, "voicePlayButton");
    const time = Check.find(panel, "voiceTimeLabel");

    if (time && time.text === "Unavailable")
      return Check.fail("the voice note reports \"Unavailable\" instead of playing");

    if (player && player.path && button && button.text === "⏸") return root.finish(player);

    stepper.attempts++;
    if (stepper.attempts >= 200) {
      return Check.fail("the voice note never started playing: path=\""
        + (player ? player.path : "?") + "\" button=\"" + (button ? button.text : "?") + "\"");
    }
    stepper.retry(root.waitForPlaying);
  }

  // finish checks the downloaded file is real Ogg audio, the fake
  // connector's own stand-in for a voice note, not a borrowed photo
  // placeholder with the wrong content for its extension.
  function finish(player: var): void {
    if (player.path.indexOf(".ogg") === -1)
      return Check.fail("downloaded voice note path \"" + player.path + "\" does not end in .ogg");

    console.log("PASS VoicePlayback");
    Qt.exit(0);
  }
}
