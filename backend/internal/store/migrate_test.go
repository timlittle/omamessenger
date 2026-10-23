// These tests use package store, not store_test, because a failing
// migration cannot be produced through the public API.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestOpen_RejectsNewerSchema(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "future.db")
	db := openRaw(t, path)
	if _, err := db.ExecContext(t.Context(), `PRAGMA user_version=1000`); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(t.Context(), path, nil); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("Open = %v, want ErrSchemaTooNew", err)
	}
}

func TestMigrate_FailedStepKeepsLastGoodVersion(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "messages.db"))
	steps := []string{`CREATE TABLE good(id INTEGER)`, `CREATE TABLE broken(`}

	if err := migrate(t.Context(), db, steps); err == nil {
		t.Fatal("migrate with a broken step succeeded")
	}

	var version int
	if err := db.QueryRowContext(t.Context(), `PRAGMA user_version`).Scan(&version); err != nil || version != 1 {
		t.Fatalf("user_version = %d (%v), want 1", version, err)
	}

	if err := migrate(t.Context(), db, steps[:1]); err != nil {
		t.Fatalf("migrate after the failure = %v, want nil", err)
	}
}

// TestMigrate_ReportsAFailureReadingTheSchemaVersion confirms migrate
// surfaces a failure to even read PRAGMA user_version, rather than
// treating it as a blank database and running every step again.
func TestMigrate_ReportsAFailureReadingTheSchemaVersion(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "messages.db"))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := migrate(t.Context(), db, migrations); err == nil {
		t.Error("migrate on a closed database = nil error, want the read failure reported")
	}
}

// TestMigrate_ReclassifiesAStoredGIFDocumentAsAnimatedVideo confirms a
// message stored before a connector learned to recognise a real GIF
// document (see telegram/media.go's and whatsapp/normalize_media.go's
// own isGIFImage) is reclassified by the migration that catches it up:
// MediaVideo, Animated, with its file name kept, so it reaches the UI's
// video view and the helper's own byte check on its next fetch, the
// same as a message received after that fix, rather than staying a
// plain file chip forever.
func TestMigrate_ReclassifiesAStoredGIFDocumentAsAnimatedVideo(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "gif.db"))
	// The GIF migration is second-to-last, one before the audio
	// migration appended after it; stop short of both so inserting the
	// pre-fix rows below still finds them unreclassified.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-2]); err != nil {
		t.Fatal(err)
	}

	insertRawMessage(t, db, "m1", `{"kind":"file","fileName":"run.GIF","size":4900}`)
	insertRawMessage(t, db, "m2", `{"kind":"file","fileName":"report.pdf","size":2000}`)
	insertRawMessage(t, db, "m3", `{"kind":"video","fileName":"clip.mp4","size":9000}`)
	insertRawMessage(t, db, "m4", "")

	if err := migrate(t.Context(), db, migrations); err != nil {
		t.Fatal(err)
	}

	gif := readRawMedia(t, db, "m1")
	if gif.Kind != domain.MediaVideo || !gif.Animated || gif.FileName != "run.GIF" {
		t.Errorf("reclassified GIF document = %+v, want MediaVideo, Animated, run.GIF", gif)
	}

	pdf := readRawMedia(t, db, "m2")
	if pdf.Kind != domain.MediaFile || pdf.Animated {
		t.Errorf("an ordinary file = %+v, want kind file, not animated", pdf)
	}

	video := readRawMedia(t, db, "m3")
	if video.Kind != domain.MediaVideo || video.Animated || video.FileName != "clip.mp4" {
		t.Errorf("an ordinary video = %+v, want unchanged", video)
	}

	// Re-running the same step on an already-reclassified database must
	// not touch it again: the migration only ever runs once per
	// database through migrate's own version gate, but the UPDATE
	// itself is idempotent too, since its WHERE clause no longer
	// matches a row it already turned into a video.
	if _, err := db.ExecContext(t.Context(), migrations[len(migrations)-2]); err != nil {
		t.Fatal(err)
	}

	gifAgain := readRawMedia(t, db, "m1")
	if gifAgain != gif {
		t.Errorf("re-running the migration changed an already-reclassified row: %+v, want %+v", gifAgain, gif)
	}
}

// TestMigrate_ReclassifiesAStoredWhatsAppAudioFileAsAudio confirms a
// WhatsApp audio message stored before this connector told an ordinary
// audio file apart from a voice note is reclassified by the migration
// that catches it up: MediaAudio, so it reaches the UI's inline player
// on its next fetch, the same as a message received after that fix,
// rather than staying a plain file chip forever. A message whose file
// name is not the helper's own fixed "audio-message.ogg" - including a
// Telegram audio file, which only ever carries the sender's own name -
// is left alone, since there is nothing safe to judge it by.
func TestMigrate_ReclassifiesAStoredWhatsAppAudioFileAsAudio(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "audio.db"))
	if err := migrate(t.Context(), db, migrations[:len(migrations)-1]); err != nil {
		t.Fatal(err)
	}

	insertRawMessage(t, db, "m1", `{"kind":"file","fileName":"audio-message.ogg","size":4096,"duration":30}`)
	insertRawMessage(t, db, "m2", `{"kind":"file","fileName":"song.mp3","size":4096}`)
	insertRawMessage(t, db, "m3", `{"kind":"voice","fileName":"voice-message.ogg","size":512}`)
	insertRawMessage(t, db, "m4", "")

	if err := migrate(t.Context(), db, migrations); err != nil {
		t.Fatal(err)
	}

	audio := readRawMedia(t, db, "m1")
	if audio.Kind != domain.MediaAudio || audio.FileName != "audio-message.ogg" || audio.Duration != 30 {
		t.Errorf("reclassified WhatsApp audio message = %+v, want MediaAudio, audio-message.ogg, duration 30", audio)
	}

	telegramAudio := readRawMedia(t, db, "m2")
	if telegramAudio.Kind != domain.MediaFile {
		t.Errorf("a Telegram audio file named only by its sender = %+v, want kind file, left alone", telegramAudio)
	}

	voice := readRawMedia(t, db, "m3")
	if voice.Kind != domain.MediaVoice {
		t.Errorf("an already-correct voice note = %+v, want kind voice, unchanged", voice)
	}

	// Re-running the same step on an already-reclassified database must
	// not touch it again: the migration only ever runs once per
	// database through migrate's own version gate, but the UPDATE
	// itself is idempotent too, since its WHERE clause no longer
	// matches a row it already turned into audio.
	if _, err := db.ExecContext(t.Context(), migrations[len(migrations)-1]); err != nil {
		t.Fatal(err)
	}

	audioAgain := readRawMedia(t, db, "m1")
	if audioAgain != audio {
		t.Errorf("re-running the migration changed an already-reclassified row: %+v, want %+v", audioAgain, audio)
	}
}

// insertRawMessage inserts a minimal message row with the given media
// JSON, bypassing the store's own encodeMedia so the test can write the
// exact pre-fix shape a connector used to produce.
func insertRawMessage(t *testing.T, db *sql.DB, id, media string) {
	t.Helper()

	_, err := db.ExecContext(t.Context(), `INSERT INTO messages(id,conversation_id,remote_id,text,outgoing,status,created,media)
		VALUES(?,'c1',?,'[File]',0,'received',1,?)`, id, id, media)
	if err != nil {
		t.Fatal(err)
	}
}

// readRawMedia reads and decodes id's stored media column directly,
// bypassing the store's own public API, which this package's own
// migration tests cannot otherwise reach a pre-migration row through.
func readRawMedia(t *testing.T, db *sql.DB, id string) domain.Media {
	t.Helper()

	var encoded string
	if err := db.QueryRowContext(t.Context(), `SELECT media FROM messages WHERE id=?`, id).Scan(&encoded); err != nil {
		t.Fatal(err)
	}

	var m domain.Media
	if err := json.Unmarshal([]byte(encoded), &m); err != nil {
		t.Fatalf("media for %s does not decode: %v", id, err)
	}

	return m
}

// openRaw opens a SQLite database without migrating it.
func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db
}
