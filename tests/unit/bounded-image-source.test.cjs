// Regression guard against a QML Image or AnimatedImage that shows a
// sender's own bytes - a thumbnail, a downloaded photo, sticker or
// GIF - with no bounded sourceSize. Without one, Qt decodes at
// whatever size the file itself declares before this UI ever gets to
// check it, so a crafted header can still make Qt allocate a huge
// bitmap even after the helper's own imagesafe package has stripped
// everything else a reviewer flagged (see docs/decisions.md): the
// cap belongs on both sides. A component showing only a value this UI
// generated itself (a locally generated QR code) is not received
// content and is not checked here. A composer's attachment preview is
// checked like any other image: its content can come from the
// clipboard, which another program controls.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// IMAGE_BLOCK matches one Image or AnimatedImage declaration's opening
// line; findImageBlocks walks forward from each match to find its own
// closing brace.
const IMAGE_BLOCK = /\b(Image|AnimatedImage)\s*\{/g;

// LOCAL_SOURCE_MARKERS appear only in a source expression built from a
// value this UI generated itself, never a sender's or the clipboard's:
// a QR code this UI rendered. A block naming one of these is never
// "received content", however its source expression is built.
const LOCAL_SOURCE_MARKERS = ['qrSource'];

// qmlFiles walks dir and returns every ".qml" file under it.
function qmlFiles(dir) {
  const found = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      found.push(...qmlFiles(full));
    } else if (entry.name.endsWith('.qml')) {
      found.push(full);
    }
  }
  return found;
}

// findImageBlocks returns every Image or AnimatedImage declaration in
// src, as its own source text from "Image {" through the matching
// closing brace, with the line it starts on.
function findImageBlocks(src) {
  const blocks = [];
  for (const m of src.matchAll(IMAGE_BLOCK)) {
    const start = m.index;
    let depth = 1;
    let i = start + m[0].length;
    while (i < src.length && depth > 0) {
      if (src[i] === '{') depth += 1;
      else if (src[i] === '}') depth -= 1;
      i += 1;
    }
    const line = src.slice(0, start).split('\n').length;
    blocks.push({ type: m[1], text: src.slice(start, i), line });
  }
  return blocks;
}

// showsReceivedContent reports whether a block's source binds to a
// sender's own bytes: an embedded base64 thumbnail, or a downloaded
// file path, and not one of LOCAL_SOURCE_MARKERS' own local files.
function showsReceivedContent(block) {
  const bindsRemoteBytes = block.includes('data:image') || block.includes('"file://"');
  return bindsRemoteBytes && !LOCAL_SOURCE_MARKERS.some((marker) => block.includes(marker));
}

// findUnboundedImages returns one "<file>:<line>" description per
// Image or AnimatedImage in src that shows received content with no
// sourceSize set.
function findUnboundedImages(file, src) {
  return findImageBlocks(src)
    .filter((block) => showsReceivedContent(block.text) && !block.text.includes('sourceSize'))
    .map((block) => `${file}:${block.line} (${block.type})`);
}

test('every Image or AnimatedImage showing received content sets a bounded sourceSize', () => {
  const offenders = qmlFiles(UI_ROOT)
    .map((file) => path.relative(UI_ROOT, file))
    .flatMap((file) => findUnboundedImages(file, fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} Image/AnimatedImage showing received content with no sourceSize, so Qt would decode at ` +
    `whatever size the file itself declares:\n  ${offenders.join('\n  ')}`);
});

test('a component showing only a value generated locally is not flagged', () => {
  const src = `
    Image {
      source: "file://" + root.qrSource
    }
  `;

  assert.deepEqual(findUnboundedImages('fixture.qml', src), []);
});

test('an attachment preview with no sourceSize is flagged: its content can come from the clipboard', () => {
  const src = `
    Image {
      source: "file://" + root.attachmentPath
      fillMode: Image.PreserveAspectCrop
    }
  `;

  assert.deepEqual(findUnboundedImages('fixture.qml', src), ['fixture.qml:2 (Image)']);
});

test('a deliberately broken expectation is caught', () => {
  const src = `
    Image {
      source: thumb ? "data:image/jpeg;base64," + thumb : ""
      fillMode: Image.PreserveAspectFit
    }
  `;

  assert.deepEqual(findUnboundedImages('fixture.qml', src), ['fixture.qml:2 (Image)']);
});

test('a block with sourceSize set is not flagged', () => {
  const src = `
    Image {
      source: thumb ? "data:image/jpeg;base64," + thumb : ""
      sourceSize.width: 128
      sourceSize.height: 128
    }
  `;

  assert.deepEqual(findUnboundedImages('fixture.qml', src), []);
});

// A bounded sourceSize alone is not enough for the composer's own
// attachment chip: a file picked from disk reaches it before
// media.attach has sanitized anything, so deciding to show a preview
// from the file's own name (what guessMediaKind used to do) would let
// a crafted file reach an Image decoder before imagesafe ever runs
// (see docs/decisions.md). This guards against that decision coming
// back: every preview must be gated on a kind the helper itself
// reported.
test('no component decides an attachment preview from the file name alone', () => {
  const offenders = qmlFiles(UI_ROOT)
    .map((file) => path.relative(UI_ROOT, file))
    .filter((file) => fs.readFileSync(path.join(UI_ROOT, file), 'utf8').includes('guessMediaKind'));

  assert.deepEqual(offenders, [],
    `${offenders.length} file(s) still decide a preview from the file name: ${offenders.join(', ')}`);
});
