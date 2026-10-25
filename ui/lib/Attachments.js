.pragma library
.import "Format.js" as Format

// Decisions about an outgoing attachment's size against the open
// conversation's own account limit: the composer's "Up to 2 GB" hint
// before a file is picked, and the attachment chip's size-and-limit
// label once one is - shown only once the file sits close to or over
// that limit, never for an ordinary small attachment.

// NEAR_LIMIT_RATIO is how close size must come to limit, as a
// fraction, before the chip says anything about it: close enough to
// be worth a heads-up, while an everyday small attachment shows
// nothing extra.
var NEAR_LIMIT_RATIO = 0.9;

// attachmentLimitHint shows limit (bytes) as the composer's hint
// before a file is picked, such as "Up to 2 GB", or "" for no known
// limit (an account whose connector is not running yet, say).
function attachmentLimitHint(limit) {
  if (!limit) {
    return '';
  }

  return `Up to ${Format.fileSize(limit)}`;
}

// isNearOrOverLimit reports whether an attachment of size bytes sits
// close to or over limit, the chip's own trigger for showing a size
// label at all. Both missing or zero answers false: there is nothing
// to warn about with no known size or limit.
function isNearOrOverLimit(size, limit) {
  if (!size || !limit) {
    return false;
  }

  return size >= limit * NEAR_LIMIT_RATIO;
}

// attachmentSizeLabel shows the attachment chip's size-and-limit
// text: "" while size sits comfortably under limit, "1.9 GB of 2 GB"
// while near it, or "2.3 GB — over the 2 GB limit" once it is over.
function attachmentSizeLabel(size, limit) {
  if (!isNearOrOverLimit(size, limit)) {
    return '';
  }

  if (size > limit) {
    return `${Format.fileSize(size)} — over the ${Format.fileSize(limit)} limit`;
  }

  return `${Format.fileSize(size)} of ${Format.fileSize(limit)}`;
}
