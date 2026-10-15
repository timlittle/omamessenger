// Checks PhotoView's "empty grey box" case: a photo with no preview
// thumbnail shows nothing while its download is still unresolved, shows
// "Photo unavailable" once that fetch is known to have failed, and, when
// a thumbnail is there after all, falls back to it instead of showing
// the unavailable message, since a stale download failure still has
// something to show. Also checks the safety net for a path that no
// longer resolves to a real file - the outgoing copy of a just-sent
// photo once the helper has retired it into its download cache, say -
// instead of a permanently broken image: the real, asynchronous load
// failure must ask for a fresh path rather than leaving the bubble
// stuck on the one that is gone.
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  // reloadWantedCount counts every "wanted" signal brokenPhoto emits,
  // standing in for the controller re-fetching its media.
  property int reloadWantedCount: 0

  property var steps: [root.checkUnavailableMessage, root.waitForBrokenPhotoToAskAgain]

  // checkUnavailableMessage is the original synchronous check: it needs
  // no waiting, so it always finishes on its first attempt.
  function checkUnavailableMessage(): var {
    const pending = Check.find(pendingPhoto, "unavailableLabel");
    const failed = Check.find(failedPhoto, "unavailableLabel");
    const failedWithThumb = Check.find(failedPhotoWithThumb, "unavailableLabel");

    if (pending.visible)
      return "a photo with no thumb and no failed fetch yet shows \"Photo unavailable\" too early";
    if (!failed.visible)
      return "a photo with no thumb and a failed fetch does not show \"Photo unavailable\"";
    if (failedWithThumb.visible)
      return "a photo with a thumb shows \"Photo unavailable\" instead of falling back to it";
    return true;
  }

  // waitForBrokenPhotoToAskAgain holds until brokenPhoto, whose path
  // names a file that does not exist, asks for a fresh one once its
  // real Image fails to load it.
  function waitForBrokenPhotoToAskAgain(): var {
    return root.reloadWantedCount >= 1;
  }

  FloatingWindow {
    implicitWidth: 400
    implicitHeight: 400
    visible: true

    Column {
      PhotoView {
        id: pendingPhoto
        photo: ({ kind: "photo", width: 4, height: 3, thumb: "" })
        failed: false
      }

      PhotoView {
        id: failedPhoto
        photo: ({ kind: "photo", width: 4, height: 3, thumb: "" })
        failed: true
      }

      PhotoView {
        id: failedPhotoWithThumb
        photo: ({ kind: "photo", width: 4, height: 3, thumb: "/9j/placeholder" })
        failed: true
      }

      PhotoView {
        id: brokenPhoto
        photo: ({ kind: "photo", width: 4, height: 3, thumb: "" })
        path: "/nonexistent/path-for-photoview-test.jpg"
        onWanted: root.reloadWantedCount++
      }
    }
  }

  Stepper {
    id: stepper
    name: "PhotoView"
    steps: root.steps
    startFn: () => stepper.runStep()
  }
}
