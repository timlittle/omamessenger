// Checks PhotoView's "empty grey box" case: a photo with no preview
// thumbnail shows nothing while its download is still unresolved, shows
// "Photo unavailable" once that fetch is known to have failed, and, when
// a thumbnail is there after all, falls back to it instead of showing
// the unavailable message, since a stale download failure still has
// something to show.
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  function run(): void {
    const pending = Check.find(pendingPhoto, "unavailableLabel");
    const failed = Check.find(failedPhoto, "unavailableLabel");
    const failedWithThumb = Check.find(failedPhotoWithThumb, "unavailableLabel");

    if (pending.visible)
      return Check.fail("a photo with no thumb and no failed fetch yet shows \"Photo unavailable\" too early");
    if (!failed.visible)
      return Check.fail("a photo with no thumb and a failed fetch does not show \"Photo unavailable\"");
    if (failedWithThumb.visible)
      return Check.fail("a photo with a thumb shows \"Photo unavailable\" instead of falling back to it");

    console.log("PASS PhotoView");
    Qt.exit(0);
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
    }
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }
}
