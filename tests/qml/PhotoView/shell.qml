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
import "ui/lib/ImageBounds.js" as ImageBounds
import "Check.js" as Check

ShellRoot {
  id: root

  // reloadWantedCount counts every "wanted" signal brokenPhoto emits,
  // standing in for the controller re-fetching its media.
  property int reloadWantedCount: 0

  property var steps: [root.checkUnavailableMessage, root.checkLyingDimensionsNeverRaiseSourceSize, root.waitForBrokenPhotoToAskAgain]

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

  // checkLyingDimensionsNeverRaiseSourceSize covers the differential
  // review's own finding: a photo's claimed width/height is the
  // sender's own, and must never reach this bubble's own sourceSize
  // binding at all, even when it describes an extreme shape that
  // disagrees wildly with the thumbnail's own real, verified size
  // (thumbWidth/thumbHeight). Proven here by recomputing
  // boundedSourceSize directly from the thumbnail's verified shape,
  // over whatever box this photo's own (lying-aspect-ratio-driven)
  // layout settled on, and requiring the component's own Image to
  // match it exactly: were the claim read instead, the two would
  // disagree, since 100000x1 and 4x3 scale to the same box in
  // entirely different shapes (see ImageBounds.verifiedDimensions and
  // docs/decisions.md).
  function checkLyingDimensionsNeverRaiseSourceSize(): var {
    const image = Check.find(lyingDimensionsPhoto, "photoImage");
    if (!image) return "could not find the lying photo's own Image";

    const want = ImageBounds.boundedSourceSize(lyingDimensionsPhoto.implicitWidth, lyingDimensionsPhoto.implicitHeight, 4, 3);
    if (image.sourceSize.width !== want.width || image.sourceSize.height !== want.height)
      return `sourceSize = ${image.sourceSize.width}x${image.sourceSize.height}, want ${want.width}x${want.height} (from the thumbnail's own verified 4x3, not the claimed 100000x1)`;
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

      PhotoView {
        id: lyingDimensionsPhoto
        // width/height claim an extreme, mismatched shape; only
        // thumbWidth/thumbHeight - the thumbnail's own verified size
        // - must ever reach this bubble's own sourceSize.
        photo: ({ kind: "photo", width: 100000, height: 1, thumbWidth: 4, thumbHeight: 3, thumb: "/9j/placeholder" })
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
