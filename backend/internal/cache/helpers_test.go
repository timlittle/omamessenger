package cache_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

// alwaysExists is a cache.MessageExists that reports every id as a real
// message, for a test that is not itself about Sweep's orphan check.
func alwaysExists(context.Context, string) (bool, error) { return true, nil }

// neverExists is a cache.MessageExists that reports no id as a real
// message, standing in for an outgoing area whose files all belong to
// deleted or never-sent messages.
func neverExists(context.Context, string) (bool, error) { return false, nil }

// fakeMessages is a cache.MessageExists backed by a set of ids, for a
// test that needs some files to be orphans and others not.
type fakeMessages struct {
	mu  sync.Mutex
	ids map[string]bool
}

func (f *fakeMessages) exists(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.ids[id], nil
}

func (f *fakeMessages) forget(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.ids, id)
}

// newFakeMessages returns a fakeMessages reporting every one of ids as a
// stored message.
func newFakeMessages(ids ...string) *fakeMessages {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}

	return &fakeMessages{ids: set}
}

// newOutgoing returns an Outgoing over a fresh temp directory with a
// grace period and size limit generous enough never to interfere with a
// test that is not itself about sweeping, and every id reported as a
// real message.
func newOutgoing(t *testing.T) *cache.Outgoing {
	t.Helper()

	return cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 24*time.Hour, 1<<30, alwaysExists)
}

// recordingLogger counts the diagnostic lines it is given, standing in
// for a *log.Logger without a real writer to parse.
type recordingLogger struct {
	mu sync.Mutex
	n  int
}

func (l *recordingLogger) Printf(string, ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.n++
}

func (l *recordingLogger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.n
}

// mustStore stores content in o under id and fileName and fails the
// test if Store returns an error, for a test that just needs a file in
// place to exercise something else, such as Sweep.
func mustStore(t *testing.T, o *cache.Outgoing, id, fileName, content string) string {
	t.Helper()

	path, err := o.Store(t.Context(), id, fileName, strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	return path
}

// backdate sets path's modification time back by d, so a sweep sees it
// as older than it actually is.
func backdate(t *testing.T, path string, d time.Duration) {
	t.Helper()

	old := time.Now().Add(-d)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

// failingReader always fails to read, like a clipboard command that
// could not be run.
type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

// blockingReader hands back chunk once, closing started right before
// doing so, then blocks until resume is closed before reporting EOF: a
// deterministic way to hold a Store (or Ensure) call open long enough
// for a second call racing the same key to prove whether it waits,
// without relying on timing or a large payload to make a real race
// likely.
type blockingReader struct {
	chunk   []byte
	started chan struct{}
	resume  chan struct{}
	sent    bool
}

func (r *blockingReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		close(r.started)

		return copy(p, r.chunk), nil
	}

	<-r.resume

	return 0, io.EOF
}
