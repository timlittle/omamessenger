// Regression guard against the automatic-fetch class a security
// review found: a photo, sticker or voice note's own row asked for its
// full file as soon as it became visible - never a click - but the
// path that fetch took (MediaController.fetchMedia, reached from
// PhotoView, StickerView and VoiceNotePlayer's own "wanted()" signal
// through MessageBubbleContent, MessageDelegate, MessengerLayout and
// ConversationController.fetchMedia) sent media.fetch with no auto
// flag at all, so the helper applied its far larger, deliberate-click
// cap to a fetch the user never asked for. fetchMedia must always pass
// auto true; retryMedia, the tap-to-load action offered once that
// smaller cap refuses something, must always pass auto false, the
// same deliberate fetch a click already gets. These checks are
// deliberately simple substring checks, not a generic parser, since
// MediaController.qml's own downloadMedia(timeline, id, report, auto,
// done) call shape never varies at these two call sites.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const MEDIA_CONTROLLER = path.resolve(__dirname, '../../ui/controllers/MediaController.qml');
const MESSAGE_BUBBLE_CONTENT = path.resolve(__dirname, '../../ui/components/MessageBubbleContent.qml');

// functionBody returns name's own function body in src, up to (not
// including) the next function declaration at the same indent, or the
// end of the file.
function functionBody(src, name) {
  const start = src.indexOf(`function ${name}(`);
  assert.ok(start >= 0, `function ${name} not found in ${src === undefined ? '?' : 'file'}`);

  const next = src.indexOf('\n  function ', start + 1);
  return next < 0 ? src.slice(start) : src.slice(start, next);
}

test('fetchMedia always passes auto true to downloadMedia', () => {
  const body = functionBody(fs.readFileSync(MEDIA_CONTROLLER, 'utf8'), 'fetchMedia');

  assert.ok(body.includes('downloadMedia(timeline, id, false, true, () => {});'),
    'fetchMedia must call downloadMedia with auto: true, so the visibility-triggered fetch gets the small, automatic cap');
});

test('retryMedia always passes auto false to downloadMedia', () => {
  const body = functionBody(fs.readFileSync(MEDIA_CONTROLLER, 'utf8'), 'retryMedia');

  assert.ok(body.includes('downloadMedia(timeline, id, false, false, () => {});'),
    'retryMedia must call downloadMedia with auto: false, the same deliberate fetch a click already gets');
});

test('every photo, sticker and voice note wanted() handler reaches mediaWanted(), never a different signal', () => {
  const src = fs.readFileSync(MESSAGE_BUBBLE_CONTENT, 'utf8');
  const handlers = [...src.matchAll(/onWanted:\s*(.+)/g)].map((m) => m[1].trim());

  assert.ok(handlers.length >= 3, `want at least 3 onWanted handlers (photo, sticker, voice note), found ${handlers.length}`);
  for (const handler of handlers) {
    assert.equal(handler, 'root.mediaWanted()', `onWanted handler "${handler}" does not reach mediaWanted()`);
  }
});

test('a deliberately broken expectation is caught: fetchMedia without auto true', () => {
  const body = 'function fetchMedia(timeline: var, id: string): void {\n    root.downloadMedia(timeline, id, false, false, () => {});\n  }';

  assert.ok(!body.includes('downloadMedia(timeline, id, false, true, () => {});'));
});
