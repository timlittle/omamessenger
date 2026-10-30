// Tests for ui/lib/ImageBounds.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const ImageBounds = load('lib/ImageBounds.js');

test('fits a known, extreme aspect ratio inside the box instead of covering it', () => {
  // The decode-bomb shape from the security review: a 1x1024 thumbnail
  // asked to fill a 320x400 bubble. Qt's own PreserveAspectFit/Crop
  // scaling covers a box by scaling to the image's longer side, which
  // for this shape means a sourceSize of 320x400 would make Qt decode
  // around 320x327680 (105 megapixels). Passing the real dimensions in
  // keeps the result inside the box on both sides.
  const got = ImageBounds.boundedSourceSize(320, 400, 1, 1024);

  assert.ok(got.width <= 320, `width ${got.width} exceeds the box`);
  assert.ok(got.height <= 400, `height ${got.height} exceeds the box`);
  assert.deepEqual(got, { width: 1, height: 400 });
});

test('never upscales a media smaller than its box', () => {
  const got = ImageBounds.boundedSourceSize(320, 400, 100, 100);

  assert.deepEqual(got, { width: 100, height: 100 });
});

test('scales a normal, wider-than-box photo down to fit, preserving aspect', () => {
  const got = ImageBounds.boundedSourceSize(320, 400, 2000, 1000);

  assert.deepEqual(got, { width: 320, height: 160 });
});

test('scales a normal, taller-than-box photo down to fit, preserving aspect', () => {
  const got = ImageBounds.boundedSourceSize(320, 400, 1000, 2000);

  assert.deepEqual(got, { width: 200, height: 400 });
});

test('falls back to the box itself when the media has no known dimensions', () => {
  assert.deepEqual(ImageBounds.boundedSourceSize(320, 400, 0, 0), { width: 320, height: 400 });
  assert.deepEqual(ImageBounds.boundedSourceSize(320, 400, undefined, undefined), { width: 320, height: 400 });
  assert.deepEqual(ImageBounds.boundedSourceSize(320, 400, -1, 50), { width: 320, height: 400 });
});

test('never returns less than one pixel on either side', () => {
  const got = ImageBounds.boundedSourceSize(320, 400, 1, 100000);

  assert.ok(got.width >= 1 && got.height >= 1, `got ${JSON.stringify(got)}, want at least 1x1`);
});

test('floors a box with no size of its own to 1x1 rather than 0x0', () => {
  assert.deepEqual(ImageBounds.boundedSourceSize(0, 0, 100, 100), { width: 1, height: 1 });
});

test('a deliberately broken expectation is caught', () => {
  const got = ImageBounds.boundedSourceSize(320, 400, 1, 1024);

  assert.notDeepEqual(got, { width: 320, height: 400 });
});

// verifiedDimensions: a differential review found every caller above
// fed boundedSourceSize a message's own claimed media.width/height,
// which a sender fully controls and need not match what the actual
// file decodes to - a lying claim produces the same decode-bomb shape
// these tests already cover, just reached a different way. The fix is
// never reading that claim for sizing a decode: once the full file is
// fetched, MediaController records the helper's own verified size as
// verifiedWidth/verifiedHeight (fields the message protocol never
// sends, so they cannot collide with mediaIdentityChanged's own use
// of width/height to tell one attachment from another - see
// ui/lib/Timeline.js); before that, only the thumbnail's own verified
// thumbWidth/thumbHeight, never the claim, is safe to use.

test('verifiedDimensions reads the full file size once it is fetched', () => {
  const media = { width: 400, height: 300, verifiedWidth: 390, verifiedHeight: 290, thumbWidth: 40, thumbHeight: 30 };

  assert.deepEqual(ImageBounds.verifiedDimensions(media, true), { width: 390, height: 290 });
});

test('verifiedDimensions reads the thumbnail size before the full file is fetched', () => {
  const media = { width: 400, height: 300, verifiedWidth: 390, verifiedHeight: 290, thumbWidth: 40, thumbHeight: 30 };

  assert.deepEqual(ImageBounds.verifiedDimensions(media, false), { width: 40, height: 30 });
});

test('verifiedDimensions never falls back to a claimed size nothing has verified yet', () => {
  // No verifiedWidth/verifiedHeight or thumbWidth/thumbHeight at all:
  // a message with no thumbnail, or whose full file has not been
  // fetched yet. width/height must never be read here - they are the
  // sender's own claim for as long as the message exists - so this
  // must report unknown (0, 0), which boundedSourceSize already
  // treats as "use the box", not the claim.
  const media = { width: 100000, height: 1 };

  assert.deepEqual(ImageBounds.verifiedDimensions(media, true), { width: 0, height: 0 });
  assert.deepEqual(ImageBounds.verifiedDimensions(media, false), { width: 0, height: 0 });
});

test('verifiedDimensions reports unknown for no media at all', () => {
  assert.deepEqual(ImageBounds.verifiedDimensions(null, true), { width: 0, height: 0 });
  assert.deepEqual(ImageBounds.verifiedDimensions(undefined, false), { width: 0, height: 0 });
});
