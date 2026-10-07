package whatsapp

// openMediaStore and mediaStorePath are unexported, with no public way to
// open the real database, so this test reaches into the package rather
// than through Connector's exported API. It uses a real SQLite database
// in a temporary directory.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMediaStorePath_NamesTheAccountsDatabase(t *testing.T) {
	t.Parallel()

	got := mediaStorePath("/data/whatsapp", "wa-1")
	if want := filepath.Join("/data/whatsapp", "wa-1-media.db"); got != want {
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
		DirectPath: "/v/abc", MediaKey: []byte{1, 2, 3}, FileSHA256: []byte{4, 5},
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

func TestMediaStore_PutReplacesAnEarlierReferenceForTheSameMessage(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	first := mediaRef{DirectPath: "/v/old", MediaKey: []byte{1}, FileSHA256: []byte{1}, FileEncSHA256: []byte{1}, FileLength: 1, Mimetype: "image/jpeg"}
	second := mediaRef{DirectPath: "/v/new", MediaKey: []byte{2}, FileSHA256: []byte{2}, FileEncSHA256: []byte{2}, FileLength: 2, Mimetype: "image/jpeg"}

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
