// Checks the first-run path: with no helper installed the window says so
// and offers to install it; pressing Install downloads the release named
// in helper-version (a fake release the test builds from the test helper,
// served over file://), verifies it, and the helper starts with the fake
// accounts. The runner provides the launcher without bin/dev for this test,
// and points OMA_RELEASE_BASE at the release directory.
import QtQuick
import Quickshell
import Quickshell.Io
import "ui"

ShellRoot {
  id: root

  property int step: 0
  property int attempts: 0
  readonly property string testRoot: String(Qt.resolvedUrl(".")).replace("file://", "")
  property var steps: [root.waitForMissing, root.buildRelease, root.waitForRelease, root.pressInstall, root.waitForReady]

  // fail stops the test with a reason on stderr.
  function fail(reason: string): void {
    console.error("FAIL step " + root.step + ": " + reason + " (status " + helperService.status + ": " + helperService.detail + ")");
    Qt.exit(1);
  }

  // find returns the descendant of item with the given objectName.
  function find(item: var, name: string): var {
    if (!item) return null;
    if (item.objectName === name) return item;

    for (const child of (item.data || item.children || [])) {
      const found = root.find(child, name);
      if (found) return found;
    }
    return null;
  }

  // waitForMissing holds until the launcher reports no helper and the
  // window offers to install one.
  function waitForMissing(): var {
    const button = root.find(panel, "installButton");
    return helperService.status === "missing" && button && button.visible;
  }

  // buildRelease publishes the dev binary as a release with its checksum.
  function buildRelease(): var {
    releaseBuilder.running = true;
    return true;
  }

  // waitForRelease holds until the release directory is complete.
  function waitForRelease(): var {
    if (releaseBuilder.running) return false;
    return releaseBuilder.exitCode === 0 ? true : "building the fake release failed";
  }

  // pressInstall clicks Install helper.
  function pressInstall(): var {
    root.find(panel, "installButton").clicked();
    return true;
  }

  // waitForReady holds until the installed helper runs and the fake
  // conversations arrive.
  function waitForReady(): var {
    const list = root.find(panel, "conversationListView");
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

  // Publishes the test helper as this machine's release asset.
  Process {
    id: releaseBuilder

    property int exitCode: -1

    workingDirectory: root.testRoot
    command: ["sh", "-c", [
      "set -eu",
      "case $(uname -m) in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; esac",
      "mkdir -p release",
      "cp \"$OMA_FAKE_HELPER\" \"release/oma-messenger-service-linux-$arch\"",
      "cd release && sha256sum oma-messenger-service-linux-$arch > SHA256SUMS"
    ].join("\n")]
    onExited: code => releaseBuilder.exitCode = code
  }

  Timer {
    id: stepTimer

    interval: 100
    onTriggered: root.runStep()
  }

  // Start once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 50
    onTriggered: {
      panel.open("{}");
      root.runStep();
    }
  }
}
