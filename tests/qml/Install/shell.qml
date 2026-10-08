// Checks the automatic install path: with no helper installed, nothing is
// downloaded until the window is opened; opening it then installs and
// starts the helper with no click. A release with a bad checksum shows the
// error with a Retry button instead; the button takes keyboard focus on
// its own, Ctrl+R reaches it for real (checked against the still-bad
// release, which just fails the same way again, a safe way to prove the
// key works without disturbing the rest of the flow), and Escape still
// works rather than being stuck on whatever context installFailed left
// behind. Clicking Retry once a good release is published then succeeds.
// The runner provides the launcher without bin/dev for this test, and
// points OMA_RELEASE_BASE at the release directory.
import QtQuick
import QtTest
import Quickshell
import Quickshell.Io
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  // installingCount counts every time status becomes "installing",
  // caught through the signal rather than polled: the local release
  // check can finish well inside one 100ms poll, so polling for that
  // transient state directly could miss it even though it happened.
  property int installingCount: 0
  // _installingBefore is installingCount's value just before the
  // keyboard retry, so waitForRetrying can tell a fresh attempt from
  // the first one that already happened.
  property int _installingBefore: 0
  readonly property string testRoot: String(Qt.resolvedUrl(".")).replace("file://", "")
  property var steps: [root.waitUnopenedAndMissing, root.buildBadRelease, root.waitForBadRelease,
    root.openWindow, root.waitForInstallFailed, root.checkRetryFocused, root.checkGlobalShortcutsStillWork,
    root.retryWithKeyboard, root.waitForRetrying, root.waitForInstallFailedAgain,
    root.fixRelease, root.waitForFixedRelease, root.clickRetry, root.waitForReady]

  // waitUnopenedAndMissing holds for a second with the window never
  // opened, then checks the launcher reported no helper installed and
  // that nothing tried to download one: no release exists yet, so an
  // install attempt would fail with its own, different message.
  function waitUnopenedAndMissing(): var {
    if (helperService.status !== "missing") return false;
    if (helperService.detail.indexOf("could not download") !== -1)
      return "an install was attempted before the window was ever opened";
    if (stepper.attempts < 10) return false;
    return true;
  }

  // buildBadRelease publishes a release whose SHA256SUMS does not match
  // the binary, so the installer must refuse it.
  function buildBadRelease(): var {
    badReleaseBuilder.running = true;
    return true;
  }

  // waitForBadRelease holds until the bad release is written.
  function waitForBadRelease(): var {
    if (badReleaseBuilder.running) return false;
    return badReleaseBuilder.exitCode === 0 ? true : "building the bad release failed";
  }

  // openWindow opens the panel, the one user action this flow needs; the
  // install itself must start on its own.
  function openWindow(): var {
    panel.open("{}");
    return true;
  }

  // waitForInstallFailed holds until the checksum mismatch is reported and
  // the window offers Retry as the only way on.
  function waitForInstallFailed(): var {
    const button = Check.find(panel, "retryButton");
    if (helperService.status !== "installFailed") return false;
    if (helperService.detail.indexOf("checksum mismatch") === -1) return false;
    return button && button.visible ? true : "installFailed but no visible Retry button";
  }

  // checkRetryFocused holds until the Retry button has taken keyboard
  // focus on its own: Panel.qml defers that a turn of the event loop
  // (see its helperInstallFailed handler), so this polls rather than
  // checking once.
  function checkRetryFocused(): var {
    const button = Check.find(panel, "retryButton");
    return !!(button && button.activeFocus);
  }

  // checkGlobalShortcutsStillWork proves installFailed is not a keyboard
  // dead end: Escape still opens and cancels the close question, the
  // same as it does once the helper is ready.
  function checkGlobalShortcutsStillWork(): var {
    t.keyClick(Qt.Key_Escape);
    const question = Check.find(panel, "closeConfirm");
    if (!question || !question.visible) return "Escape did not show the close question while installFailed";

    t.keyClick(Qt.Key_Escape);
    if (question.visible) return "Escape did not cancel the close question";
    if (!Check.find(panel, "panelWindow").visible) return "cancelling the close question left the window hidden";
    return true;
  }

  // retryWithKeyboard presses Ctrl+R against the still-bad release: a
  // safe way to prove the key reaches Retry for real, since it only
  // leads back to the same failure, which the next two steps confirm by
  // watching a fresh "installing" transition happen and settle back on
  // installFailed.
  function retryWithKeyboard(): var {
    root._installingBefore = root.installingCount;
    t.keyClick(Qt.Key_R, Qt.ControlModifier);
    return true;
  }

  // waitForRetrying holds until Ctrl+R's own attempt is under way, caught
  // through installingCount rather than polling helperService.status
  // directly: the local release check can settle back to installFailed
  // well inside one poll, so polling for "installing" itself could miss
  // it even though it really happened.
  function waitForRetrying(): var {
    return root.installingCount > root._installingBefore;
  }

  // waitForInstallFailedAgain holds until the keyboard-triggered retry
  // has failed the same way as the first attempt.
  function waitForInstallFailedAgain(): var {
    return helperService.status === "installFailed" && helperService.detail.indexOf("checksum mismatch") !== -1;
  }

  // fixRelease rewrites SHA256SUMS with the binary's real checksum.
  function fixRelease(): var {
    goodReleaseBuilder.running = true;
    return true;
  }

  // waitForFixedRelease holds until the corrected release is written.
  function waitForFixedRelease(): var {
    if (goodReleaseBuilder.running) return false;
    return goodReleaseBuilder.exitCode === 0 ? true : "fixing the release failed";
  }

  // clickRetry presses the Retry button shown after the failed install.
  function clickRetry(): var {
    Check.find(panel, "retryButton").clicked();
    return true;
  }

  // waitForReady holds until the installed helper runs and the fake
  // conversations arrive.
  function waitForReady(): var {
    const list = Check.find(panel, "conversationListView");
    return helperService.status === "ready" && list && list.count === 11;
  }

  Service {
    id: helperService
  }

  Panel {
    id: panel

    service: helperService
    shell: QtObject {
      function hide(id) {}
    }
  }

  TestCase {
    id: t
    when: false
  }

  // Catches every "installing" transition through the signal itself,
  // since it can come and go faster than a poll would ever see.
  Connections {
    target: helperService
    function onStatusChanged() {
      if (helperService.status === "installing") root.installingCount++;
    }
  }

  // Publishes the test helper as this machine's release asset, with a
  // SHA256SUMS that does not match it.
  Process {
    id: badReleaseBuilder

    property int exitCode: -1

    workingDirectory: root.testRoot
    command: ["sh", "-c", [
      "set -eu",
      "case $(uname -m) in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; esac",
      "mkdir -p release",
      "cp \"$OMA_FAKE_HELPER\" \"release/oma-messenger-service-linux-$arch\"",
      "printf '%064d  oma-messenger-service-linux-%s\\n' 0 \"$arch\" > release/SHA256SUMS"
    ].join("\n")]
    onExited: code => badReleaseBuilder.exitCode = code
  }

  // Rewrites SHA256SUMS with the binary's real checksum, leaving the rest
  // of the release as badReleaseBuilder published it.
  Process {
    id: goodReleaseBuilder

    property int exitCode: -1

    workingDirectory: root.testRoot
    command: ["sh", "-c", [
      "set -eu",
      "case $(uname -m) in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; esac",
      "cd release && sha256sum oma-messenger-service-linux-$arch > SHA256SUMS"
    ].join("\n")]
    onExited: code => goodReleaseBuilder.exitCode = code
  }

  // The window is deliberately left closed here: the first step must
  // see nothing download without it.
  Stepper {
    id: stepper
    name: "Install"
    steps: root.steps
    maxAttempts: 200
    describeFailure: (reason) => reason + " (status " + helperService.status + ": " + helperService.detail + ")"
    startFn: () => stepper.runStep()
  }
}
