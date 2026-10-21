package whatsapp

// openMediaStore and mediaStorePath are unexported, with no public way to
// open the real database, so this test reaches into the package rather
// than through Connector's exported API. It uses a real SQLite database
// in a temporary directory.

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMediaStorePath_NamesTheAccountsDatabase(t *testing.T) {
	t.Parallel()

	got := mediaStorePath("/data/whatsapp", "wa-1")
	if want := "/data/whatsapp/wa-1-media.db"; got != want {
		t.Errorf("mediaStorePath = %q, want %q", got, want)
	}
}

func TestOpenMediaStore_CreatesAPrivateFreshDatabase(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "whatsapp")
	store, err := openMediaStore(t.Context(), dir, "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.close() }()

	info, err := os.Stat(mediaStorePath(dir, "wa-1"))
	if err != nil {
		t.Fatalf("media store file was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("media store file mode = %o, want 0600", perm)
	}
}

func TestMediaStore_PutThenGetRoundTrips(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	ref := mediaRef{
		Kind: mediaKindImage, DirectPath: "/v/abc", MediaKey: []byte{1, 2, 3}, FileSHA256: []byte{4, 5},
		FileEncSHA256: []byte{6, 7}, FileLength: 42, Mimetype: "image/jpeg",
	}

	if err := store.put(t.Context(), "conv-1", "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.get(t.Context(), "conv-1", "msg-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("get after put = not found, want found")
	}
	if !reflect.DeepEqual(got, ref) {
		t.Errorf("get = %+v, want %+v", got, ref)
	}
}

func TestMediaStore_GetReportsNotFoundForAnUnknownMessage(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	_, ok, err := store.get(t.Context(), "conv-1", "nope")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("get for an unsaved message = found, want not found")
	}
}

// TestOpenMediaStore_RejectsAnUnwritableDirectory confirms a directory
// this process cannot write into, such as one owned by another user or
// left behind with the wrong permissions, is reported as an error
// rather than a silent, broken store.
func TestOpenMediaStore_RejectsAnUnwritableDirectory(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions, as in a local act run")
	}

	dir := filepath.Join(t.TempDir(), "whatsapp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }()

	if _, err := openMediaStore(t.Context(), dir, "wa-1"); err == nil {
		t.Error("openMediaStore in an unwritable directory = nil error, want one")
	}
}

// TestOpenMediaStore_RejectsAFileLeftCorrupted confirms a media store
// file that is not a valid SQLite database - left behind by a crash or
// a disk that filled up mid-write - is reported as an error rather
// than treated as an empty store, which would otherwise silently lose
// every reference already saved in it.
func TestOpenMediaStore_RejectsAFileLeftCorrupted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(mediaStorePath(dir, "wa-1"), []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := openMediaStore(t.Context(), dir, "wa-1"); err == nil {
		t.Error("openMediaStore over a corrupted file = nil error, want one")
	}
}

func TestMediaStore_PutReplacesAnEarlierReferenceForTheSameMessage(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	first := mediaRef{Kind: mediaKindImage, DirectPath: "/v/old", MediaKey: []byte{1}, FileSHA256: []byte{1}, FileEncSHA256: []byte{1}, FileLength: 1, Mimetype: "image/jpeg"}
	second := mediaRef{Kind: mediaKindVideo, DirectPath: "/v/new", MediaKey: []byte{2}, FileSHA256: []byte{2}, FileEncSHA256: []byte{2}, FileLength: 2, Mimetype: "image/jpeg"}

	if err := store.put(t.Context(), "conv-1", "msg-1", first); err != nil {
		t.Fatal(err)
	}
	if err := store.put(t.Context(), "conv-1", "msg-1", second); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.get(t.Context(), "conv-1", "msg-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !reflect.DeepEqual(got, second) {
		t.Errorf("get after replace = %+v, %v, want %+v, true", got, ok, second)
	}
}

// TestOpenMediaStoreDSN_AddsFileNameToAnOlderSchema covers an
// account's database from before a document's file name was needed to
// tell a real GIF image apart from an ordinary document (see
// imageProfile): CREATE TABLE IF NOT EXISTS alone never adds a column
// to a table that already exists under the older schema.
func TestOpenMediaStoreDSN_AddsFileNameToAnOlderSchema(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const oldSchema = `CREATE TABLE media_refs (
		conversation_id TEXT NOT NULL,
		message_id      TEXT NOT NULL,
		kind            TEXT NOT NULL DEFAULT '',
		direct_path     TEXT NOT NULL,
		media_key       BLOB NOT NULL,
		file_sha256     BLOB NOT NULL,
		file_enc_sha256 BLOB NOT NULL,
		file_length     INTEGER NOT NULL,
		mimetype        TEXT NOT NULL,
		PRIMARY KEY (conversation_id, message_id)
	)`
	if _, err := db.ExecContext(t.Context(), oldSchema); err != nil {
		t.Fatal(err)
	}
	old := mediaRef{
		Kind: mediaKindDocument, DirectPath: "/v/old", MediaKey: []byte{1}, FileSHA256: []byte{1},
		FileEncSHA256: []byte{1}, FileLength: 1, Mimetype: "application/pdf",
	}
	const insertOld = `INSERT INTO media_refs
		(conversation_id, message_id, kind, direct_path, media_key, file_sha256, file_enc_sha256, file_length, mimetype)
		VALUES ('conv-1', 'msg-old', ?, ?, ?, ?, ?, ?, ?)`
	if _, err := db.ExecContext(t.Context(), insertOld, string(old.Kind), old.DirectPath, old.MediaKey, old.FileSHA256, old.FileEncSHA256, old.FileLength, old.Mimetype); err != nil {
		t.Fatal(err)
	}

	if err := addMediaRefFileNameColumn(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	m := &mediaStore{db: db}
	got, ok, err := m.get(t.Context(), "conv-1", "msg-old")
	if err != nil || !ok || got.FileName != "" {
		t.Errorf("get(row from before the migration) = %+v, %v, %v, want FileName empty, found, no error", got, ok, err)
	}

	gif := mediaRef{
		Kind: mediaKindDocument, DirectPath: "/v/gif", MediaKey: []byte{2}, FileSHA256: []byte{2},
		FileEncSHA256: []byte{2}, FileLength: 2, Mimetype: "image/gif", FileName: "nice.gif",
	}
	if err := m.put(t.Context(), "conv-1", "msg-gif", gif); err != nil {
		t.Fatal(err)
	}
	got, ok, err = m.get(t.Context(), "conv-1", "msg-gif")
	if err != nil || !ok || !reflect.DeepEqual(got, gif) {
		t.Errorf("get after migration = %+v, %v, %v, want %+v, true, no error", got, ok, err, gif)
	}
}
