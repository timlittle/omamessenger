// Tests for ui/lib/Media.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Media = load('lib/Media.js');

test('kindFor returns null for no media', () => {
  assert.strictEqual(Media.kindFor(null), null);
  assert.strictEqual(Media.kindFor(undefined), null);
  assert.strictEqual(Media.kindFor({}), null);
});

test('kindFor passes through photo, video, link, voice, audio and sticker unchanged', () => {
  assert.strictEqual(Media.kindFor({ kind: 'photo' }), 'photo');
  assert.strictEqual(Media.kindFor({ kind: 'video' }), 'video');
  assert.strictEqual(Media.kindFor({ kind: 'link' }), 'link');
  assert.strictEqual(Media.kindFor({ kind: 'voice' }), 'voice');
  assert.strictEqual(Media.kindFor({ kind: 'audio' }), 'audio');
  assert.strictEqual(Media.kindFor({ kind: 'sticker' }), 'sticker');
});

test('kindFor leaves a legacy voice note stored as a file as a plain file', () => {
  // Before the helper's "voice" media kind existed, a WhatsApp push-to-talk
  // message was stored as a plain file named "voice-message.ogg"; that
  // stored row never gets re-normalized. A sender's claimed file name must
  // never choose a decoder (see docs/decisions.md), so this is no longer
  // guessed back into "voice" from its name - it opens only through the
  // gated external-open path, like any other file.
  const media = { kind: 'file', fileName: 'voice-message.ogg', duration: 7 };
  assert.strictEqual(Media.kindFor(media), 'file');
});

test('kindFor never guesses voice from a file name, for any audio extension', () => {
  for (const name of ['clip.mp3', 'clip.m4a', 'clip.wav', 'clip.aac', 'clip.opus', 'clip.oga', 'clip.amr', 'clip.weba', 'clip.flac']) {
    assert.strictEqual(Media.kindFor({ kind: 'file', fileName: name }), 'file', name);
  }
});

test('kindFor leaves a non-audio file as a plain file', () => {
  assert.strictEqual(Media.kindFor({ kind: 'file', fileName: 'report.pdf' }), 'file');
  assert.strictEqual(Media.kindFor({ kind: 'file', fileName: 'archive.zip' }), 'file');
});

test('kindFor treats a file with no name as a plain file', () => {
  assert.strictEqual(Media.kindFor({ kind: 'file' }), 'file');
});

test('kindFor never answers photo or video for a file kind', () => {
  // Guards the bug this fixes: an audio file must never reach the image
  // view, which would try to decode it as a photo and fail.
  const media = { kind: 'file', fileName: 'voice-message.ogg' };
  const result = Media.kindFor(media);
  assert.notStrictEqual(result, 'photo');
  assert.notStrictEqual(result, 'video');
});

test('isAnimated is true only for a video the service marked animated', () => {
  assert.strictEqual(Media.isAnimated({ kind: 'video', animated: true }), true);
  assert.strictEqual(Media.isAnimated({ kind: 'video', animated: false }), false);
  assert.strictEqual(Media.isAnimated({ kind: 'video' }), false);
  assert.strictEqual(Media.isAnimated({ kind: 'photo', animated: true }), false);
  assert.strictEqual(Media.isAnimated(null), false);
  assert.strictEqual(Media.isAnimated(undefined), false);
});

test('thumbMime recognizes a PNG thumb by its own base64-encoded signature', () => {
  // The base64 encoding of PNG's fixed 8-byte signature, exactly what
  // every PNG store.sanitizedThumb ever writes starts with, whatever
  // real pixels follow.
  assert.strictEqual(Media.thumbMime('iVBORw0KGgoAAAANSUhEUgAA'), 'image/png');
});

test('thumbMime defaults to JPEG for anything that is not a PNG, including no thumb at all', () => {
  // The base64 encoding of JPEG's fixed start-of-image marker.
  assert.strictEqual(Media.thumbMime('/9j/4AAQSkZJRgABAQAAAQ=='), 'image/jpeg');
  assert.strictEqual(Media.thumbMime(''), 'image/jpeg');
  assert.strictEqual(Media.thumbMime(null), 'image/jpeg');
  assert.strictEqual(Media.thumbMime(undefined), 'image/jpeg');
});

test('mimeFor names a photo as JPEG and a video as MP4', () => {
  assert.strictEqual(Media.mimeFor({ kind: 'photo' }), 'image/jpeg');
  assert.strictEqual(Media.mimeFor({ kind: 'video' }), 'video/mp4');
});

test('mimeFor is null for a kind with nothing sensible to copy', () => {
  for (const kind of ['link', 'voice', 'audio', 'sticker', 'poll', 'file']) {
    assert.strictEqual(Media.mimeFor({ kind }), null, kind);
  }
  assert.strictEqual(Media.mimeFor(null), null);
  assert.strictEqual(Media.mimeFor(undefined), null);
});

test('decodesAsGif is true only for the helper\'s own "gif" verdict', () => {
  assert.strictEqual(Media.decodesAsGif('gif'), true);
});

test('decodesAsGif is false for anything else, including a name or flag that merely claims gif', () => {
  // Guards the bug this fixes: a sender fully controls a video's
  // fileName and animated flag, so neither one may ever decide this -
  // only media.fetch's own decodeAs result can.
  assert.strictEqual(Media.decodesAsGif(''), false);
  assert.strictEqual(Media.decodesAsGif('video'), false);
  assert.strictEqual(Media.decodesAsGif(null), false);
  assert.strictEqual(Media.decodesAsGif(undefined), false);
});

test('canPlayInline is true for a genuine GIF image regardless of playable', () => {
  assert.strictEqual(Media.canPlayInline('gif', false), true);
  assert.strictEqual(Media.canPlayInline('gif', true), true);
  assert.strictEqual(Media.canPlayInline('gif', undefined), true);
});

test('canPlayInline is true for an ordinary video only once playable is true', () => {
  assert.strictEqual(Media.canPlayInline('', true), true);
  assert.strictEqual(Media.canPlayInline('', false), false);
  assert.strictEqual(Media.canPlayInline('', undefined), false);
});

test('copyable is true for a photo regardless of decodeAs or playable', () => {
  assert.strictEqual(Media.copyable({ kind: 'photo' }, '', false), true);
  assert.strictEqual(Media.copyable({ kind: 'photo' }, 'gif', true), true);
});

test('copyable is true for a video only once the helper confirms it can play, never from the sender\'s own claim alone', () => {
  // Guards the bug this fixes: the UI used to offer copy for any
  // message merely claiming to be a video, never checking the
  // helper's own verdict the way the view that actually plays it
  // already does (see docs/decisions.md).
  assert.strictEqual(Media.copyable({ kind: 'video' }, '', true), true);
  assert.strictEqual(Media.copyable({ kind: 'video' }, '', false), false);
  assert.strictEqual(Media.copyable({ kind: 'video' }, '', undefined), false);
});

test('copyable is true for a video the helper confirms is a genuine GIF, regardless of playable', () => {
  assert.strictEqual(Media.copyable({ kind: 'video' }, 'gif', false), true);
});

test('copyable is false for every kind mimeFor already refuses', () => {
  for (const kind of ['link', 'voice', 'audio', 'sticker', 'poll', 'file']) {
    assert.strictEqual(Media.copyable({ kind }, '', true), false, kind);
  }
  assert.strictEqual(Media.copyable(null, '', true), false);
  assert.strictEqual(Media.copyable(undefined, '', true), false);
});
