// Checks LinkOpenConfirm: the real host a mismatched link opens shows
// on its own line, in full, with no elide at all, however long a
// userinfo padding engineered to look like a different, convincing
// domain sits before the real "@"; the full URL shown below it elides
// only in the middle, never the end a security review found hid that
// same host completely. A mailto: link, which carries no separate
// authority host, hides that line rather than showing an empty one.
//
// A later review found a backslash inside the authority makes
// urlHost's own split (on "/", "?" or "#" only) disagree with a
// browser's own WHATWG parser (which also ends an authority at "\"),
// so this also checks a URL Links.isAllowed now refuses outright
// shows this dialog's own refusal state instead - no "Really opens:"
// line computed from a host this cannot trust, no Open or Copy choice
// to act on it, just the one short message and a way to close it.
import QtQuick
import Quickshell
import qs.Commons
import "ui/components"
import "ui/lib/Links.js" as Links
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

    root.checkRefusedLinkShowsNoHostAndNoOpenChoice();
  }

  // checkRefusedLinkShowsNoHostAndNoOpenChoice confirms a URL
  // Links.isAllowed refuses outright - the review's own backslash
  // case, which made urlHost read "paypal.com" while a browser would
  // have opened "evil.example" - never reaches that wrong host at
  // all: no emphasis line computed from it, the fixed refusal message
  // as the description instead of "It will open:", and no Open or
  // Copy choice left for a stray keypress to land on.
  function checkRefusedLinkShowsNoHostAndNoOpenChoice(): void {
    const target = "https://evil.example\\@paypal.com/login";
    if (Links.isAllowed(target)) return Check.fail("test setup is wrong: this target should already be disallowed");
    dialog.target = target;

    const emphasis = Check.find(dialog, "dialogEmphasis");
    if (emphasis && emphasis.visible)
      return Check.fail("the emphasis line shows a host computed from a refused URL: " + JSON.stringify(emphasis.text));

    const description = Check.find(dialog, "dialogDescription");
    if (!description) return Check.fail("no dialogDescription line found");
    if (description.text !== Links.REFUSED_LINK_MESSAGE)
      return Check.fail("description = " + JSON.stringify(description.text) + ", want the refusal message");

    if (Check.find(dialog, "openButton")) return Check.fail("a refused link still offers an Open anyway choice");
    if (Check.find(dialog, "copyButton")) return Check.fail("a refused link still offers a Copy link choice");
    if (!Check.find(dialog, "cancelButton")) return Check.fail("a refused link has no way to close the dialog");

    console.log("PASS LinkOpenConfirm");
    Qt.exit(0);
  }
}
