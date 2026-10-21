// Regression guard against a QML MediaPlayer decoding a downloaded
// video or voice note with nothing checking its validated bytes
// first. Every real MediaPlayer this UI owns lives inside
// VideoRuntime.qml or VoiceNoteRuntime.qml, loaded through a Loader
// only QtMultimedia-optional code touches (see each file's own
// comment); the gate itself - whether a real file path ever reaches
// that player - lives one layer up, at whichever file actually
// triggers playback with a real path, never inside the runtime files
// themselves, which are deliberately dumb decoders trusted by their
// caller.
//
// This file checks both halves: that no new MediaPlayer appears
// outside the two reviewed runtime files, and that every known place
// that actually starts real playback - setting a loaded runtime's
// own `source` to a real path, or calling VoiceNoteController's
// `toggle` - reads the helper's own `playable` verdict (see
// media.fetch's playable in docs/api.md) somewhere in that same
// file, never the sender's claimed file name or kind.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// RUNTIME_FILES are the only files allowed to declare a real
// MediaPlayer at all. A new one appearing anywhere else needs the
// same review this file's own two entries already got.
const RUNTIME_FILES = ['components/VoiceNoteRuntime.qml', 'components/VideoRuntime.qml'];

// PLAYBACK_TRIGGERS name every known way a real file path reaches one
// of RUNTIME_FILES' players: assigning a loaded VideoRuntime's own
// `source` (InlineVideo's and FullVideoPlayer's `onLoaded` handler),
// and calling VoiceNoteController.toggle with a real path
// (MediaController._selectOrOpenVoice). Each pattern's `files` is the
// exact, exhaustive set of files allowed to match it today - an exact
// list, not just a lower bound, so removing the last caller here is
// noticed too, the same way openurl-coverage.test.cjs's own allow-lists
// work.
const PLAYBACK_TRIGGERS = [
  { pattern: /_runtime\.item\.source\s*=\s*root\.path/, files: ['components/InlineVideo.qml', 'components/FullVideoPlayer.qml'] },
  { pattern: /voiceController\.toggle\(/, files: ['controllers/MediaController.qml'] },
];

// qmlFiles walks dir and returns every ".qml" file under it, as paths
// relative to UI_ROOT.
function qmlFiles(dir) {
  const found = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      found.push(...qmlFiles(full));
    } else if (entry.name.endsWith('.qml')) {
      found.push(path.relative(UI_ROOT, full));
    }
  }
  return found;
}

const ALL_FILES = qmlFiles(UI_ROOT);
const CONTENTS = new Map(ALL_FILES.map((file) => [file, fs.readFileSync(path.join(UI_ROOT, file), 'utf8')]));

test('every MediaPlayer lives in a reviewed runtime file', () => {
  const offenders = ALL_FILES.filter((file) => /\bMediaPlayer\s*\{/.test(CONTENTS.get(file)) && !RUNTIME_FILES.includes(file));

  assert.deepEqual(offenders, [],
    `MediaPlayer found outside the reviewed runtime files: ${offenders.join(', ')}. ` +
    'A new one needs the same playable-gating review RUNTIME_FILES already got.');
});

for (const { pattern, files } of PLAYBACK_TRIGGERS) {
  test(`every caller of ${pattern.source} is a known, playable-gated file`, () => {
    const matching = ALL_FILES.filter((file) => pattern.test(CONTENTS.get(file)));

    assert.deepEqual(matching.sort(), [...files].sort(),
      `expected exactly ${JSON.stringify(files)} to trigger real playback this way, found ${JSON.stringify(matching)}`);

    for (const file of matching) {
      assert.ok(CONTENTS.get(file).includes('playable'),
        `${file} triggers real playback but never reads the helper's own playable verdict`);
    }
  });
}

test('a deliberately broken expectation is caught', () => {
  const offenders = ['components/InlineVideo.qml'].filter((file) => !RUNTIME_FILES.includes(file));
  assert.notDeepEqual(offenders, []);
});
