// Regression guard against a message bubble that freezes the shell's UI
// thread decoding media it never shows. A security review found two
// separate causes, both fixed together (see docs/decisions.md):
//
// 1. MessageBubbleContent used to create every media view (link preview,
//    photo, video, file, sticker, poll, voice note) for every message,
//    merely hiding the ones that did not match the message's own kind.
//    A hidden Image still loads its source, so a batch of heavy-but-
//    within-limits progressive JPEG thumbnails froze the desktop on
//    open, decoded by views that were never shown at all. The fix wraps
//    each view in its own Loader, active only for the matching kind, so
//    a message's media never reaches a view it does not belong to.
// 2. LinkPreview's own Image had no `asynchronous: true`, so even a
//    visible link preview decoded its thumbnail on the shell's thread
//    rather than a worker thread, unlike every other Image showing a
//    sender's bytes.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');
const MESSAGE_BUBBLE_CONTENT = path.join(UI_ROOT, 'components/MessageBubbleContent.qml');

// IMAGE_BLOCK matches one Image or AnimatedImage declaration's opening
// line; findBlocks walks forward from each match to find its own closing
// brace, the same way bounded-image-source.test.cjs does.
const IMAGE_BLOCK = /\b(Image|AnimatedImage)\s*\{/g;

// LOCAL_SOURCE_MARKERS appear only in a source expression built from a
// value this UI generated itself, never a sender's or the clipboard's.
const LOCAL_SOURCE_MARKERS = ['qrSource'];

// MEDIA_VIEW_TYPES are the bubble's own media views, one per message
// kind: each must be built only inside a Loader gated on that kind, never
// as a plain child every message creates.
const MEDIA_VIEW_TYPES = ['LinkPreview', 'PhotoView', 'InlineVideo', 'FileView', 'StickerView', 'PollView', 'VoiceNotePlayer'];

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

// findBlocks returns every declaration in src whose opening line matches
// pattern, as its own source text through the matching closing brace,
// with the line it starts on and the index its body starts at.
function findBlocks(src, pattern) {
  const blocks = [];
  for (const m of src.matchAll(pattern)) {
    const start = m.index;
    let depth = 1;
    let i = start + m[0].length;
    while (i < src.length && depth > 0) {
      if (src[i] === '{') depth += 1;
      else if (src[i] === '}') depth -= 1;
      i += 1;
    }
    const line = src.slice(0, start).split('\n').length;
    blocks.push({ type: m[1], text: src.slice(start, i), start, line });
  }
  return blocks;
}

// showsReceivedContent reports whether a block's source binds to a
// sender's own bytes: an embedded base64 thumbnail, or a downloaded file
// path, and not one of LOCAL_SOURCE_MARKERS' own local files.
function showsReceivedContent(block) {
  const bindsRemoteBytes = block.includes('data:image') || block.includes('"file://"');
  return bindsRemoteBytes && !LOCAL_SOURCE_MARKERS.some((marker) => block.includes(marker));
}

// findSyncImages returns one "<file>:<line>" description per Image (never
// AnimatedImage, which has no such property) in src that shows received
// content without `asynchronous: true`, so Qt would decode it on
// whichever thread calls into it - the shell's own UI thread, for
// anything built directly rather than through a worker.
function findSyncImages(file, src) {
  return findBlocks(src, IMAGE_BLOCK)
    .filter((block) => block.type === 'Image' && showsReceivedContent(block.text) && !block.text.includes('asynchronous: true'))
    .map((block) => `${file}:${block.line}`);
}

// findUngatedAnimatedImages returns one "<file>:<line>" description per
// AnimatedImage in src with no "playing" property anywhere in its own
// declaration: AnimatedImage has no asynchronous property, so the only
// defence against it decoding every frame of a sender's GIF for free is
// never instantiating or playing it until the user has chosen to.
function findUngatedAnimatedImages(file, src) {
  return findBlocks(src, IMAGE_BLOCK)
    .filter((block) => block.type === 'AnimatedImage' && !block.text.includes('playing'))
    .map((block) => `${file}:${block.line}`);
}

// ancestorLabels returns the named QML block types still open at index
// (excluding the block starting at index itself): the identifier
// immediately before each unclosed "{" that src has reached by then.
// Braces inside a value, such as an empty object literal "({})", have no
// preceding identifier and contribute a null label, which is filtered
// out; they still have to be pushed and popped to keep depth correct.
function ancestorLabels(src, index) {
  const stack = [];
  for (let i = 0; i < index; i++) {
    const ch = src[i];
    if (ch === '{') {
      let j = i - 1;
      while (j >= 0 && /\s/.test(src[j])) j -= 1;
      const end = j + 1;
      while (j >= 0 && /[A-Za-z0-9_.]/.test(src[j])) j -= 1;
      const start = j + 1;
      stack.push(start < end ? src.slice(start, end) : null);
    } else if (ch === '}') {
      stack.pop();
    }
  }
  return stack.filter(Boolean);
}

// stripLineComments blanks out "// ..." to the end of each line, so a
// comment's own prose (which can contain stray braces) never confuses
// ancestorLabels' brace counting. A "//" is only treated as a comment
// when it starts the line or follows whitespace, so a URL such as
// "https://..." inside a string is left alone.
function stripLineComments(src) {
  return src.replace(/(^|\s)\/\/.*$/gm, '$1');
}

// findUngatedMediaViews returns one "<file>:<line> (<Type>)" description
// per MEDIA_VIEW_TYPES declaration in src that is not nested inside some
// Loader: a view built as a Loader's child only exists once that Loader
// is active, so it is the one way to guarantee a message's media never
// reaches a view that does not match its kind.
function findUngatedMediaViews(file, src) {
  const clean = stripLineComments(src);
  const typePattern = new RegExp(`\\b(${MEDIA_VIEW_TYPES.join('|')})\\s*\\{`, 'g');
  const offenders = [];
  for (const m of clean.matchAll(typePattern)) {
    const line = clean.slice(0, m.index).split('\n').length;
    if (!ancestorLabels(clean, m.index).includes('Loader')) offenders.push(`${file}:${line} (${m[1]})`);
  }
  return offenders;
}

test('every Image showing received content decodes asynchronously', () => {
  const offenders = qmlFiles(UI_ROOT)
    .map((file) => path.relative(UI_ROOT, file))
    .flatMap((file) => findSyncImages(file, fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} Image showing received content with no "asynchronous: true", so Qt would decode it on whichever ` +
    `thread calls in:\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught: a synchronous Image showing a thumbnail', () => {
  const src = `
    Image {
      source: thumb ? "data:image/jpeg;base64," + thumb : ""
    }
  `;

  assert.deepEqual(findSyncImages('fixture.qml', src), ['fixture.qml:2']);
});

test('an asynchronous Image showing a thumbnail is not flagged', () => {
  const src = `
    Image {
      source: thumb ? "data:image/jpeg;base64," + thumb : ""
      asynchronous: true
    }
  `;

  assert.deepEqual(findSyncImages('fixture.qml', src), []);
});

test('a locally generated Image, such as a QR code, is never flagged', () => {
  const src = `
    Image {
      source: Setup.qrSource(root.qr)
    }
  `;

  assert.deepEqual(findSyncImages('fixture.qml', src), []);
});

test('every AnimatedImage is gated on the user having chosen to play it', () => {
  const offenders = qmlFiles(UI_ROOT)
    .map((file) => path.relative(UI_ROOT, file))
    .flatMap((file) => findUngatedAnimatedImages(file, fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} AnimatedImage with no "playing" gate, so it could animate a sender's GIF without the user choosing ` +
    `to play it:\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught: an AnimatedImage with no playing gate', () => {
  const src = `
    AnimatedImage {
      source: visible ? Links.fileURL(root.path) : ""
    }
  `;

  assert.deepEqual(findUngatedAnimatedImages('fixture.qml', src), ['fixture.qml:2']);
});

test('MessageBubbleContent builds each media view only inside a Loader gated on its kind', () => {
  const src = fs.readFileSync(MESSAGE_BUBBLE_CONTENT, 'utf8');
  const offenders = findUngatedMediaViews('components/MessageBubbleContent.qml', src);

  assert.deepEqual(offenders, [],
    `${offenders.length} media view built outside a Loader, so every message would create it - and decode its media - ` +
    `whether or not it matches that view's own kind:\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught: a media view built as a plain child', () => {
  const src = `
    Column {
      PhotoView {
        photo: root.media
      }
    }
  `;

  assert.deepEqual(findUngatedMediaViews('fixture.qml', src), ['fixture.qml:3 (PhotoView)']);
});

test('a media view nested inside a Loader is not flagged', () => {
  const src = `
    Column {
      Loader {
        active: root._mediaKind === "photo"
        sourceComponent: Component {
          PhotoView {
            photo: root.media
          }
        }
      }
    }
  `;

  assert.deepEqual(findUngatedMediaViews('fixture.qml', src), []);
});

test('a comment mentioning a media view type by name is not mistaken for a declaration', () => {
  const src = `
    Column {
      // PhotoView { used to be a plain child here before the fix }
      Loader {
        active: root._mediaKind === "photo"
        sourceComponent: Component {
          PhotoView {
            photo: root.media
          }
        }
      }
    }
  `;

  assert.deepEqual(findUngatedMediaViews('fixture.qml', src), []);
});
