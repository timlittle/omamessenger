.pragma library

// ImageBounds works out a sourceSize for a QML Image or AnimatedImage
// that is guaranteed to stay inside its own box, never cover it, so
// Qt never decodes more pixels than the box can show: a lying or
// mismatched declared size otherwise lets Qt's own cover scaling
// allocate a bitmap many times the box's area (see docs/decisions.md).

// boundedSourceSize returns the sourceSize {width, height} to give an
// Image or AnimatedImage drawn in a boxWidth by boxHeight box, for
// media whose real size is mediaWidth by mediaHeight. The result
// always fits inside the box and never upscales past the media's own
// size; an unknown media size falls back to the box itself.
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
// height to feed boundedSourceSize, never media's own sender-claimed
// width/height (see docs/decisions.md). haveFile selects the full
// file's verified size once downloaded, or the thumbnail's verified
// size before that. Returns {width: 0, height: 0} when neither is
// known yet.
function verifiedDimensions(media, haveFile) {
  if (!media) {
    return { width: 0, height: 0 };
  }
  if (haveFile) {
    return { width: media.verifiedWidth || 0, height: media.verifiedHeight || 0 };
  }

  return { width: media.thumbWidth || 0, height: media.thumbHeight || 0 };
}
