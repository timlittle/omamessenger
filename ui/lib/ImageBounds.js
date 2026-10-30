.pragma library

// ImageBounds works out a sourceSize for a QML Image or AnimatedImage
// that is guaranteed to stay inside its own box - never cover it - so
// Qt never decodes more than that box's own pixel count. Qt's own
// PreserveAspectFit/PreserveAspectCrop scaling works out a decode size
// that covers a requested sourceSize box: it takes the larger of the
// width and height ratios between the box and the image's own real
// size, and can scale up. When sourceSize is set to the box itself but
// the real image is a very different shape (a 1x1024 thumbnail asked
// to fill a 320x400 bubble), that scaling allocates a bitmap many
// times the box's own area, which is the decode-bomb shape a crafted
// thumbnail header can still carry after imagesafe's own aspect-ratio
// cap, since a sender's declared width and height need not match what
// the UI already has from the message's own metadata (see
// docs/decisions.md). Computing sourceSize from the media's own known
// width and height instead, scaled down (never up) to fit inside the
// box on both sides, keeps the ratio Qt is asked for the same as the
// image's own real ratio, so its "cover" scaling never amplifies past
// the box at all.

// boundedSourceSize returns the sourceSize {width, height} to give an
// Image or AnimatedImage drawn in a boxWidth by boxHeight box, for
// media whose own real size is mediaWidth by mediaHeight. The result
// always fits inside the box on both sides and never upscales past
// the media's own size. mediaWidth or mediaHeight that is not a
// positive number - unknown, as a link preview's thumbnail never
// carries its own dimensions - falls back to the box itself, exactly
// the bound every caller used before this function existed. Every
// side is floored at one pixel, since a box or an image can be
// reported as zero while still needing a concrete size to pass to
// Qt.
function boundedSourceSize(boxWidth, boxHeight, mediaWidth, mediaHeight) {
  const box = { width: Math.max(1, boxWidth), height: Math.max(1, boxHeight) };

  if (!(mediaWidth > 0) || !(mediaHeight > 0)) {
    return box;
  }

  const scale = Math.min(box.width / mediaWidth, box.height / mediaHeight, 1);
  const width = Math.max(1, Math.round(mediaWidth * scale));
  const height = Math.max(1, Math.round(mediaHeight * scale));

  return { width, height };
}

// verifiedDimensions returns the real, decoder-confirmed width and
// height a caller must feed boundedSourceSize instead of media's own
// claimed width/height: a later differential review found that field
// is exactly what a sender controls, and a claim that disagrees with
// the real file's own shape reproduces the same decode-bomb
// boundedSourceSize exists to close, just reached through the
// metadata rather than the box (see docs/decisions.md). haveFile is
// true once media's full photo or sticker has been downloaded and
// sanitized - the moment MediaController records media.fetch's own
// verified size as verifiedWidth/verifiedHeight, fields the message
// protocol never sends and so cannot collide with anything a sender
// claims. Before that, only the thumbnail's own verified
// thumbWidth/thumbHeight - set the same way, from the thumbnail bytes
// themselves - are safe to read. media's own width/height are never
// read here at all: they stay the sender's unverified claim for as
// long as the message exists, kept only for display (see
// ui/lib/Format.js's photoSize) and for telling one attachment apart
// from another. Returns {width: 0, height: 0}, which
// boundedSourceSize already treats as "use the box", when media is
// missing or carries no verified size yet.
function verifiedDimensions(media, haveFile) {
  if (!media) {
    return { width: 0, height: 0 };
  }
  if (haveFile) {
    return { width: media.verifiedWidth || 0, height: media.verifiedHeight || 0 };
  }

  return { width: media.thumbWidth || 0, height: media.thumbHeight || 0 };
}
