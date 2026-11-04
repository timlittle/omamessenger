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

  // testRoot is this test's own directory, where opaque.jpg and
  // mismatched.jpg (committed alongside this file) actually live.
  readonly property string testRoot: String(Qt.resolvedUrl(".")).replace("file://", "")

  property var steps: [
    root.checkUnavailableMessage,
    root.checkLyingDimensionsNeverRaiseSourceSize,
    root.checkUnverifiedPathNeverReachesTheImage,
    root.waitForBrokenPhotoToAskAgain,
    root.waitForRealJpegThumbToRender,
    root.waitForMatchedExtensionFileToRender,
    root.waitForMismatchedExtensionFileToRender
  ]

  // realJpegThumbBase64 is a tiny, real, opaque 4x3 JPEG: imagesafe.Reencode
  // writes a thumb this way whenever it finds no real transparency (see
  // docs/decisions.md), so this proves Media.thumbMime's own sniff picks
  // "image/jpeg" and that Qt actually decodes the result, not just that
  // the Image element exists.
  readonly property string realJpegThumbBase64: "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAMCAgICAgMCAgIDAwMDBAYEBAQEBAgGBgUGCQgKCgkICQkKDA8MCgsOCwkJDRENDg8QEBEQCgwSExIQEw8QEBD/2wBDAQMDAwQDBAgEBAgQCwkLEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBD/wAARCAADAAQDAREAAhEBAxEB/8QAFAABAAAAAAAAAAAAAAAAAAAAB//EABQQAQAAAAAAAAAAAAAAAAAAAAD/xAAUAQEAAAAAAAAAAAAAAAAAAAAH/8QAFBEBAAAAAAAAAAAAAAAAAAAAAP/aAAwDAQACEQMRAD8ANzAL3//Z"

  // waitForRealJpegThumbToRender polls realJpegThumbPhoto's own Image
  // until it leaves the loading state: Ready and a positive sourceSize
  // means Qt actually decoded it; Error, or any other mismatch, fails
  // the test outright rather than retrying out the clock.
  function waitForRealJpegThumbToRender(): var {
    const image = Check.find(realJpegThumbPhoto, "photoImage");
    if (!image) return "could not find realJpegThumbPhoto's own Image";
    if (image.status === Image.Error) return "real JPEG thumb failed to decode";
    if (image.status !== Image.Ready) return false;
    if (image.sourceSize.width <= 0 || image.sourceSize.height <= 0) return "real JPEG thumb decoded to an empty size";
    return true;
  }

  // waitForMatchedExtensionFileToRender polls matchedExtensionPhoto's own
  // Image: a real JPEG file named "opaque.jpg", the ordinary case a
  // display copy's own name always matches (see
  // connector.ReencodeImageFile and docs/decisions.md).
  function waitForMatchedExtensionFileToRender(): var {
    const image = Check.find(matchedExtensionPhoto, "photoImage");
    if (!image) return "could not find matchedExtensionPhoto's own Image";
    if (image.status === Image.Error) return "matched-extension JPEG file failed to decode";
    if (image.status !== Image.Ready) return false;
    if (image.sourceSize.width <= 0 || image.sourceSize.height <= 0) return "matched-extension JPEG file decoded to an empty size";
    return true;
  }

  // waitForMismatchedExtensionFileToRender polls mismatchedExtensionPhoto's
  // own Image: real PNG bytes in a file named "mismatched.jpg", proving
  // Qt's own QImageReader sniffs a file:// source's actual content rather
  // than trusting its suffix - the question docs/decisions.md needed an
  // answer to before deciding an outgoing attachment's own on-disk name
  // never needs to change just because imagesafe.Reencode may have
  // changed its format.
  function waitForMismatchedExtensionFileToRender(): var {
    const image = Check.find(mismatchedExtensionPhoto, "photoImage");
    if (!image) return "could not find mismatchedExtensionPhoto's own Image";
    if (image.status === Image.Error) return "mismatched-extension PNG file failed to decode despite its .jpg name";
    if (image.status !== Image.Ready) return false;
    if (image.sourceSize.width <= 0 || image.sourceSize.height <= 0) return "mismatched-extension PNG file decoded to an empty size";
    return true;
  }

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

  // checkUnverifiedPathNeverReachesTheImage covers the fix for a
  // security review's media-identity race: a media.fetch answer served
  // for one kind can land on a row that reads as a photo again by the
  // time it arrives, even though MediaController's own staleness check
  // is meant to catch it (see ui/controllers/MediaController.qml). As
  // a second, independent check, this view itself must never decode a
  // path unless its own verifiedWidth/verifiedHeight - set only once
  // MediaController has recorded media.fetch's result for this exact
  // photo - are there too: a path with no verified size at all, even
  // though it is set and looks like any other path, falls back to the
  // thumbnail instead of ever reaching the Image.
  function checkUnverifiedPathNeverReachesTheImage(): var {
    const image = Check.find(unverifiedPhoto, "photoImage");
    if (!image) return "could not find unverifiedPhoto's own Image";
    if (String(image.source).includes("opaque.jpg")) {
      return "the Image decoded an unverified path instead of falling back to the thumbnail";
    }
    if (!String(image.source).startsWith("data:image/jpeg;base64,")) {
      return "the Image source = " + image.source + ", want the thumbnail data URL";
    }
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
        id: unverifiedPhoto
        // path names a real, decodable JPEG - the same opaque.jpg
        // matchedExtensionPhoto below loads successfully - so this
        // only proves the fix, not a path that would have failed to
        // decode anyway. No verifiedWidth/verifiedHeight, exactly as
        // it would be for a mismatched-kind answer that slipped past
        // MediaController's own check: this view must still refuse to
        // decode it, even though it is a perfectly real image file.
        photo: ({ kind: "photo", width: 4, height: 3, thumb: root.realJpegThumbBase64 })
        path: root.testRoot + "/opaque.jpg"
      }

      PhotoView {
        id: brokenPhoto
        // verifiedWidth/verifiedHeight stand in for what
        // MediaController records once media.fetch actually confirms
        // a photo's display copy (see PhotoView._haveFile): required
        // here too, now that the view never decodes a path without
        // them, even one that turns out not to exist on disk.
        photo: ({ kind: "photo", width: 4, height: 3, verifiedWidth: 4, verifiedHeight: 3, thumb: "" })
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

      PhotoView {
        id: realJpegThumbPhoto
        photo: ({ kind: "photo", width: 4, height: 3, thumb: root.realJpegThumbBase64 })
      }

      PhotoView {
        id: matchedExtensionPhoto
        photo: ({ kind: "photo", width: 4, height: 3, verifiedWidth: 4, verifiedHeight: 3, thumb: "" })
        path: root.testRoot + "/opaque.jpg"
      }

      PhotoView {
        id: mismatchedExtensionPhoto
        photo: ({ kind: "photo", width: 8, height: 8, verifiedWidth: 8, verifiedHeight: 8, thumb: "" })
        path: root.testRoot + "/mismatched.jpg"
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
