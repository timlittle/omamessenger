package main

// medialegacy_test.go covers revalidateLegacyMedia: the one-time sweep
// that closes the gap a plain cache hit would otherwise leave for a
// file written by a version of this helper that predates one of
// imagesafe's own checks (see docs/decisions.md). It wires the same
// mediaCaches serve itself builds, through newMediaCaches, so the
// test exercises exactly the startup path a real upgrade goes through.

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/store"
)

// onePixelPNG returns a tiny, real PNG that imagesafe already accepts
// unchanged.
func onePixelPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// pngWithOversizedIHDR returns a tiny, real PNG whose IHDR is
// rewritten to declare a far larger width and height than it actually
// encodes: the same bomb imagesafe.Sanitize's own dimension check
// exists to reject, standing in for a file a pre-sanitizer version of
// this helper once wrote unchecked.
func pngWithOversizedIHDR() []byte {
	out := append([]byte{}, onePixelPNG()...)
	binary.BigEndian.PutUint32(out[16:20], 50_000)
	binary.BigEndian.PutUint32(out[20:24], 50_000)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(out[12:29]))
	copy(out[29:33], crc[:])
	return out
}

// newTestMediaCaches returns the same mediaCaches serve itself builds,
// rooted at a fresh temporary directory (also returned, so a test can
// plant a file directly where the downloaded media cache keeps its
// own) with a real, empty store behind the outgoing area's own orphan
// check.
func newTestMediaCaches(t *testing.T) (mediaCaches, string) {
	t.Helper()

	dataDir := t.TempDir()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "messages.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return newMediaCaches(dataDir, db), dataDir
}

// TestRevalidateLegacyMedia_SanitizesCachedImages covers the
// downloaded media cache's own half of the sweep: a legacy file that
// still fails today's checks is removed, one that only carries
// something today's sanitizer strips is kept and rewritten, and a
// file that is not an image at all is left completely alone.
func TestRevalidateLegacyMedia_SanitizesCachedImages(t *testing.T) {
	t.Parallel()

	caches, dataDir := newTestMediaCaches(t)
	dir := filepath.Join(dataDir, "media")
	bad := filepath.Join(dir, "bad.jpg")
	good := filepath.Join(dir, "good.jpg")
	other := filepath.Join(dir, "video.mp4")
	mustWrite(t, bad, pngWithOversizedIHDR())
	mustWrite(t, good, onePixelPNG())
	mustWrite(t, other, []byte("not an image at all"))

	revalidateLegacyMedia(caches, log.New(os.Stderr, "", 0))

	assertGone(t, bad)
	assertPresent(t, good)
	assertUnchanged(t, other, []byte("not an image at all"))
}

// TestRevalidateLegacyMedia_SanitizesOutgoingAttachments covers the
// outgoing attachment area's own half of the sweep, through the exact
// full-size sanitizer every outgoing attachment already gets today.
func TestRevalidateLegacyMedia_SanitizesOutgoingAttachments(t *testing.T) {
	t.Parallel()

	caches, _ := newTestMediaCaches(t)
	bad := caches.outgoing.Path("m1", "bad.jpg")
	good := caches.outgoing.Path("m2", "good.jpg")
	other := caches.outgoing.Path("m3", "notes.txt")
	mustWrite(t, bad, pngWithOversizedIHDR())
	mustWrite(t, good, onePixelPNG())
	mustWrite(t, other, []byte("plain text attachment"))

	revalidateLegacyMedia(caches, log.New(os.Stderr, "", 0))

	assertGone(t, bad)
	assertPresent(t, good)
	assertUnchanged(t, other, []byte("plain text attachment"))
}

// TestRevalidateLegacyMedia_RunsOnceAndRecordsItsOwnMarker covers the
// format marker both areas gain once the sweep has run: a file
// planted only after the marker is already current is left alone the
// next time the helper starts, since there is nothing left for the
// marker to say is out of date.
func TestRevalidateLegacyMedia_RunsOnceAndRecordsItsOwnMarker(t *testing.T) {
	t.Parallel()

	caches, _ := newTestMediaCaches(t)
	logger := log.New(os.Stderr, "", 0)
	revalidateLegacyMedia(caches, logger)

	downloadedVersion, err := caches.downloaded.FormatVersion()
	if err != nil || downloadedVersion != legacyMediaFormatVersion {
		t.Fatalf("downloaded FormatVersion() = %d, %v, want %d, nil", downloadedVersion, err, legacyMediaFormatVersion)
	}
	outgoingVersion, err := caches.outgoing.FormatVersion()
	if err != nil || outgoingVersion != legacyMediaFormatVersion {
		t.Fatalf("outgoing FormatVersion() = %d, %v, want %d, nil", outgoingVersion, err, legacyMediaFormatVersion)
	}

	// Planted after the marker is already current: a second run must
	// not touch it, since nothing told it this file is from an older
	// version at all.
	laterBad := caches.outgoing.Path("m4", "later.jpg")
	mustWrite(t, laterBad, pngWithOversizedIHDR())

	revalidateLegacyMedia(caches, logger)

	assertPresent(t, laterBad)
}

// TestRevalidateLegacyMedia_RemovesAFileItCannotReadButLeavesTheMarkerUnset
// covers the fix for a read failure during the sweep: a legacy file
// that cannot even be opened - here, every permission bit cleared -
// is removed rather than kept just because this run failed to look at
// it, but the error that removal itself resolves still counts as the
// sweep not completing clean, so its area's format marker is left
// unset: the next start tries again, rather than ever trusting a
// start that hit an error as proof the area is now fully checked (see
// revalidateArea's own doc comment).
func TestRevalidateLegacyMedia_RemovesAFileItCannotReadButLeavesTheMarkerUnset(t *testing.T) {
	t.Parallel()
	skipAsRootInMain(t)

	caches, dataDir := newTestMediaCaches(t)
	path := filepath.Join(dataDir, "media", "unreadable.jpg")
	mustWrite(t, path, onePixelPNG())
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) }) // let t.TempDir() clean it up

	revalidateLegacyMedia(caches, log.New(os.Stderr, "", 0))

	assertGone(t, path)

	version, err := caches.downloaded.FormatVersion()
	if err != nil || version != 0 {
		t.Errorf("downloaded FormatVersion() = %d, %v, want 0, nil (unset, so the next start retries)", version, err)
	}
}

// skipAsRootInMain skips a test that relies on file permissions
// refusing access: root bypasses them, so as root (as CI's container
// runs) the failure the test expects can never happen.
func skipAsRootInMain(t *testing.T) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("file permissions do not restrict root")
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertGone(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s still exists, want it removed", path)
	}
}

func assertPresent(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s stat = %v, want it still present", path, err)
	}
}

func assertUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Errorf("%s = %q, %v, want %q, nil", path, got, err, want)
	}
}
