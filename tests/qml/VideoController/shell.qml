// Checks VideoController: toggling a video selects it, toggling the
// same one again deselects it rather than restarting it, and selecting
// a second video takes over from the first, since only one non-animated
// video plays at a time. It also checks setMuted and the fallback this
// whole feature falls back to: pointing the controller at a runtime
// file that fails to load (standing in for a machine with no
// QtMultimedia installed) reports "not available". Every check here is
// synchronous: VideoController only tracks selection and mute state,
// it never itself decodes anything.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property var steps: [
    root.checkAvailable,
    root.checkToggleSelectsAndDeselects,
    root.checkSecondVideoTakesOver,
    root.checkSetMuted,
    root.checkUnavailableFallback
  ]

  // checkAvailable verifies the real controller, loading its real
  // runtime, reports itself available: QtMultimedia is installed on
  // every machine these offscreen tests run on.
  function checkAvailable(): var {
    if (!controller.available) return "controller reports unavailable with its real runtime";
    return true;
  }

  // checkToggleSelectsAndDeselects verifies toggling a video selects
  // it, and toggling the same id again deselects it.
  function checkToggleSelectsAndDeselects(): var {
    controller.toggle("m1");
    if (controller.playingId !== "m1") return "playingId is \"" + controller.playingId + "\", want \"m1\"";

    controller.toggle("m1");
    if (controller.playingId !== "") return "toggling the selected video again did not deselect it";
    return true;
  }

  // checkSecondVideoTakesOver verifies selecting a different video
  // replaces the one selected first.
  function checkSecondVideoTakesOver(): var {
    controller.toggle("m1");
    controller.toggle("m2");
    if (controller.playingId !== "m2") return "a second video did not take over from the first";
    return true;
  }

  // checkSetMuted verifies the shared mute preference starts muted by
  // default and flips on request.
  function checkSetMuted(): var {
    if (!controller.muted) return "controller does not start muted by default";

    controller.setMuted(false);
    if (controller.muted) return "setMuted(false) did not unmute";
    return true;
  }

  // checkUnavailableFallback verifies a controller whose runtime file
  // cannot load reports itself unavailable, the path a machine without
  // QtMultimedia takes.
  function checkUnavailableFallback(): var {
    if (broken.available) return "controller reports available with a runtime file that does not exist";
    return true;
  }

  VideoController {
    id: controller
  }

  VideoController {
    id: broken
    runtimeSource: Qt.resolvedUrl("does-not-exist.qml")
  }

  Stepper {
    id: stepper
    name: "VideoController"
    steps: root.steps
    startFn: () => stepper.runStep()
  }
}
