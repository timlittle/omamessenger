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

test('kindFor passes through photo, video, link, voice and sticker unchanged', () => {
  assert.strictEqual(Media.kindFor({ kind: 'photo' }), 'photo');
  assert.strictEqual(Media.kindFor({ kind: 'video' }), 'video');
  assert.strictEqual(Media.kindFor({ kind: 'link' }), 'link');
  assert.strictEqual(Media.kindFor({ kind: 'voice' }), 'voice');
  assert.strictEqual(Media.kindFor({ kind: 'sticker' }), 'sticker');
});

test('kindFor treats a legacy voice note stored as a file as voice', () => {
  // Before the helper's "voice" media kind existed, a WhatsApp push-to-talk
  // message was stored as a plain file named "voice-message.ogg"; that
  // stored row never gets re-normalized, so this is real, permanent data.
  const media = { kind: 'file', fileName: 'voice-message.ogg', duration: 7 };
  assert.strictEqual(Media.kindFor(media), 'voice');
});

test('kindFor treats any audio file extension as voice, not just ogg', () => {
  for (const name of ['clip.mp3', 'clip.m4a', 'clip.wav', 'clip.aac', 'clip.opus', 'clip.oga', 'clip.amr', 'clip.weba', 'clip.flac']) {
    assert.strictEqual(Media.kindFor({ kind: 'file', fileName: name }), 'voice', name);
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

test('isAudioFileName is case-insensitive and ignores files with no extension', () => {
  assert.strictEqual(Media.isAudioFileName('Voice.OGG'), true);
  assert.strictEqual(Media.isAudioFileName('noext'), false);
  assert.strictEqual(Media.isAudioFileName(''), false);
  assert.strictEqual(Media.isAudioFileName(undefined), false);
});

test('isAnimated is true only for a video the service marked animated', () => {
  assert.strictEqual(Media.isAnimated({ kind: 'video', animated: true }), true);
  assert.strictEqual(Media.isAnimated({ kind: 'video', animated: false }), false);
  assert.strictEqual(Media.isAnimated({ kind: 'video' }), false);
  assert.strictEqual(Media.isAnimated({ kind: 'photo', animated: true }), false);
  assert.strictEqual(Media.isAnimated(null), false);
  assert.strictEqual(Media.isAnimated(undefined), false);
});

test('mimeFor names a photo as JPEG and a video as MP4', () => {
  assert.strictEqual(Media.mimeFor({ kind: 'photo' }), 'image/jpeg');
  assert.strictEqual(Media.mimeFor({ kind: 'video' }), 'video/mp4');
});

test('mimeFor is null for a kind with nothing sensible to copy', () => {
  for (const kind of ['link', 'voice', 'sticker', 'poll', 'file']) {
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
