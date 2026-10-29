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
