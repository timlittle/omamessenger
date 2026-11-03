// Checks LinkOpenConfirm: the real host a mismatched link opens shows
// on its own line, in full, with no elide at all, however long a
// userinfo padding engineered to look like a different, convincing
// domain sits before the real "@"; the full URL shown below it elides
// only in the middle, never the end a security review found hid that
// same host completely. A mailto: link, which carries no separate
// authority host, hides that line rather than showing an empty one.
import QtQuick
import Quickshell
import qs.Commons
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  FloatingWindow {
    id: win
    implicitWidth: Style.space(500)
    implicitHeight: Style.space(400)
    visible: true

    LinkOpenConfirm {
      id: dialog
      open: true
      // The review's own shape: a convincing fake name, padded long
      // enough that an end-elided single line would have pushed the
      // real host - evil.example, the only thing after the final "@" -
      // off the end entirely.
      target: "https://bank.example.com" + "a".repeat(1500) + "@evil.example/login"
    }
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.checkHostNeverElided()
  }

  // checkHostNeverElided confirms the emphasis line names the link's
  // real host in full, with no elide mode at all, and that the
  // description below it elides only in the middle.
  function checkHostNeverElided(): void {
    const emphasis = Check.find(dialog, "dialogEmphasis");
    if (!emphasis) return Check.fail("no dialogEmphasis line found");
    if (!emphasis.visible) return Check.fail("the emphasis line is not visible for a link with a real host");
    if (emphasis.text.indexOf("evil.example") < 0)
      return Check.fail("the emphasis line does not name the link's real host: " + JSON.stringify(emphasis.text));
    if (emphasis.elide !== Text.ElideNone)
      return Check.fail("the emphasis line elides instead of showing the host in full: elide = " + emphasis.elide);

    const description = Check.find(dialog, "dialogDescription");
    if (!description) return Check.fail("no dialogDescription line found");
    if (description.elide !== Text.ElideMiddle)
      return Check.fail("the description does not elide in the middle: elide = " + description.elide);

    root.checkMailtoHasNoSeparateHostLine();
  }

  // checkMailtoHasNoSeparateHostLine confirms a link with no separate
  // authority host (mailto:) hides the emphasis line rather than
  // showing an empty, meaningless one.
  function checkMailtoHasNoSeparateHostLine(): void {
    dialog.target = "mailto:person@example.com?body=hello";

    const emphasis = Check.find(dialog, "dialogEmphasis");
    if (emphasis && emphasis.visible)
      return Check.fail("the emphasis line shows for a mailto: link, which has no separate host");

    console.log("PASS LinkOpenConfirm");
    Qt.exit(0);
  }
}
