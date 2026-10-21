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

test('isGifFile is true only for an animated video whose name ends .gif', () => {
  assert.strictEqual(Media.isGifFile({ kind: 'video', animated: true, fileName: 'run.gif' }), true);
  assert.strictEqual(Media.isGifFile({ kind: 'video', animated: true, fileName: 'RUN.GIF' }), true);
});

test('isGifFile is false for a silent mp4 the service only marked animated', () => {
  // Every WhatsApp or Telegram "GIF" the sending service itself plays
  // back like one is actually a silent MP4, not real GIF bytes, even
  // when its own name happens to mention "gif" (see clip.gif.mp4 in
  // the connector tests).
  assert.strictEqual(Media.isGifFile({ kind: 'video', animated: true, fileName: 'clip.gif.mp4' }), false);
});

test('isGifFile is false for a non-animated video or a non-video kind', () => {
  assert.strictEqual(Media.isGifFile({ kind: 'video', animated: false, fileName: 'run.gif' }), false);
  assert.strictEqual(Media.isGifFile({ kind: 'photo', animated: true, fileName: 'run.gif' }), false);
  assert.strictEqual(Media.isGifFile(null), false);
  assert.strictEqual(Media.isGifFile(undefined), false);
});
