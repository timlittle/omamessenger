pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format
import "../lib/Media.js" as Media

// The bubble's content stack: a reply quote if this message answers one,
// then its text, then any link preview, photo or file it carries. Split
// out of MessageDelegate to keep that file within the size guideline.
// Parts a message lacks take no space.
Column {
  id: root

  // message carries text and mediaPath, as MessageDelegate reads them.
  // It, media and quote can all be reset straight to a QML null while
  // the delegate this belongs to is destroyed, so every binding below
  // guards them with "&&" or a ternary rather than trusting required to
  // mean non-null for the binding's whole lifetime.
  required property var message
  // media is what the message carries besides its text, or null.
  required property var media
  // quote is the message this one replies to, or null.
  required property var quote
  // maxTextWidth caps how wide the text, link preview, photo, file or
  // voice note may grow before wrapping or eliding.
  required property real maxTextWidth
  // voiceNotes is the playback state the caller reads from
  // VoiceNoteController: {available, playingId, positionMs, durationMs}.
  // A plain summary rather than the controller itself, so this view only
  // ever reads data, never calls into a controller.
  property var voiceNotes: ({ available: false, playingId: "", positionMs: 0, durationMs: 0 })

  // mediaWanted asks for this message's photo or voice note to be downloaded.
  signal mediaWanted()
  // mediaOpen asks for this message's photo, video, file or voice note
  // to be opened, played or paused.
  signal mediaOpen()
  // quoteOpened asks the caller to scroll to the message this one quotes.
  signal quoteOpened(string remoteId)

  // _mediaKind is which view media belongs in: "link", "photo", "video",
  // "voice", "file", or "" for no media. It is also an audio file the
  // connector only ever marked as a plain file (see Media.js), so an old
  // voice note that predates the "voice" media kind still gets a player
  // instead of the plain file row. Computed once here, rather than
  // inline on each view's own visible binding, so the path below is
  // never handed to a view other than the one that matches: an
  // invisible sibling would otherwise still try to load it, which is
  // how a voice note's audio file once reached the image view and
  // failed to decode as a photo.
  readonly property string _mediaKind: Media.kindFor(root.media) ?? ""

  spacing: Theme.spacing.xs

  ReplyQuote {
    objectName: "replyQuote"
    width: Math.min(implicitWidth, root.maxTextWidth)
    visible: !!root.quote
    reply: root.quote ? root.quote : ({})
    onOpened: remoteId => root.quoteOpened(remoteId)
  }

  TextEdit {
    id: body

    // caption leaves out the label a photo stands in for.
    readonly property string caption: root.message ? Format.caption(root.message.text, root.media) : ""

    objectName: "body"
    visible: body.caption !== ""
    width: Math.min(Math.ceil(natural.advanceWidth) + 1, root.maxTextWidth)
    readOnly: true
    selectByMouse: true
    wrapMode: TextEdit.Wrap
    textFormat: TextEdit.RichText
    text: Format.messageHtml(body.caption, Color.accent, Color.muted)
    color: Color.foreground
    font { family: Theme.font.family; pixelSize: Theme.font.body }
    onLinkActivated: link => Qt.openUrlExternally(link)
  }

  // The widest line's unwrapped width. TextEdit's own implicit width
  // follows its wrapped width, so it cannot size the bubble.
  TextMetrics {
    id: natural

    font: body.font
    text: Format.longestLine(body.caption)
  }

  LinkPreview {
    objectName: "linkPreview"
    width: Math.min(implicitWidth, root.maxTextWidth)
    visible: root._mediaKind === "link"
    preview: root.media ? root.media : ({})
    onOpened: url => Qt.openUrlExternally(url)
  }

  PhotoView {
    objectName: "photoView"
    maxWidth: root.maxTextWidth
    width: implicitWidth
    height: implicitHeight
    visible: root._mediaKind === "photo" || root._mediaKind === "video"
    photo: root.media ? root.media : ({})
    path: visible && root.message && root.message.mediaPath ? root.message.mediaPath : ""
    failed: visible && !!(root.message && root.message.mediaFailed)
    onWanted: root.mediaWanted()
    onOpened: root.mediaOpen()
  }

  FileView {
    objectName: "fileView"
    width: Math.min(implicitWidth, root.maxTextWidth)
    visible: root._mediaKind === "file"
    file: root.media ? root.media : ({})
    onOpened: root.mediaOpen()
  }

  VoiceNotePlayer {
    objectName: "voiceNotePlayer"
    width: Math.min(implicitWidth, root.maxTextWidth)
    visible: root._mediaKind === "voice"
    media: root.media ? root.media : ({})
    path: visible && root.message && root.message.mediaPath ? root.message.mediaPath : ""
    failed: visible && !!(root.message && root.message.mediaFailed)
    available: root.voiceNotes.available
    playing: root.voiceNotes.available && !!root.message && root.voiceNotes.playingId === root.message.id
    positionMs: playing ? root.voiceNotes.positionMs : 0
    durationMs: playing ? root.voiceNotes.durationMs : 0
    onWanted: root.mediaWanted()
    onOpened: root.mediaOpen()
  }
}
