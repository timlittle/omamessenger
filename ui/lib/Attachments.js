.pragma library
.import "Format.js" as Format

// Checks an outgoing attachment's size against the account limit, for
// the composer's size hint and the attachment chip's size label.

// NEAR_LIMIT_RATIO is how close size must come to limit before the
// chip mentions it.
var NEAR_LIMIT_RATIO = 0.9;

// attachmentLimitHint shows limit as the composer's hint, e.g. "Up to 2 GB".
function attachmentLimitHint(limit) {
  if (!limit) {
    return '';
  }

  return `Up to ${Format.fileSize(limit)}`;
}

// isNearOrOverLimit reports whether size sits close to or over limit.
function isNearOrOverLimit(size, limit) {
  if (!size || !limit) {
    return false;
  }

  return size >= limit * NEAR_LIMIT_RATIO;
}

// attachmentSizeLabel shows the attachment chip's size-and-limit text.
function attachmentSizeLabel(size, limit) {
  if (!isNearOrOverLimit(size, limit)) {
    return '';
  }

  if (size > limit) {
    return `${Format.fileSize(size)} — over the ${Format.fileSize(limit)} limit`;
  }

  return `${Format.fileSize(size)} of ${Format.fileSize(limit)}`;
}
