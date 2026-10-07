// Checks the automatic install path: with no helper installed, nothing is
// downloaded until the window is opened; opening it then installs and
// starts the helper with no click. A release with a bad checksum shows the
// error with a Retry button instead, and clicking Retry once a good release
// is published succeeds. The runner provides the launcher without bin/dev
// for this test, and points OMA_RELEASE_BASE at the release directory.
import QtQuick
import Quickshell
import Quickshell.Io
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property int step: 0
  property int attempts: 0
  readonly property string testRoot: String(Qt.resolvedUrl(".")).replace("file://", "")
  property var steps: [root.waitUnopenedAndMissing, root.buildBadRelease, root.waitForBadRelease,
    root.openWindow, root.waitForInstallFailed, root.fixRelease, root.waitForFixedRelease,
    root.clickRetry, root.waitForReady]

  // fail stops the test with a reason on stderr.
  function fail(reason: string): void {
    console.error("FAIL step " + root.step + ": " + reason + " (status " + helperService.status + ": " + helperService.detail + ")");
    Qt.exit(1);
  }

  // waitUnopenedAndMissing holds for a second with the window never
  // opened, then checks the launcher reported no helper installed and
  // that nothing tried to download one: no release exists yet, so an
  // install attempt would fail with its own, different message.
  function waitUnopenedAndMissing(): var {
    if (helperService.status !== "missing") return false;
    if (helperService.detail.indexOf("could not download") !== -1)
      return "an install was attempted before the window was ever opened";
    if (root.attempts < 10) return false;
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

  // runStep runs the current step and advances, retries or fails.
  function runStep(): void {
    const result = root.steps[root.step]();
    if (typeof result === "string") return root.fail(result);

    if (result === true) {
      root.step++;
      root.attempts = 0;
      if (root.step === root.steps.length) {
        console.log("PASS Install");
        return Qt.exit(0);
      }
    } else if (++root.attempts > 200) {
      return root.fail("condition not met within 20 s");
    }

    stepTimer.start();
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

  Timer {
    id: stepTimer

    interval: 100
    onTriggered: root.runStep()
  }

  // Start once Quickshell has finished loading; Qt.exit() is ignored
  // before then. The window is deliberately left closed here: the first
  // step must see nothing download without it.
  Timer {
    running: true
    interval: 50
    onTriggered: root.runStep()
  }
}
