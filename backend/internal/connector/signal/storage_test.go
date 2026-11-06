package signal

// storage_test.go reaches into the package rather than through
// Connector's exported API, since openMediaStore and its tables have
// no public surface. It uses a real SQLite database in a temporary
// directory, as privacy.md requires.

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
)

func newTestMediaStore(t *testing.T) *mediaStore {
	t.Helper()

	store, err := openMediaStore(t.Context(), filepath.Join(t.TempDir(), "signal"), "acc1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.close() })

	return store
}

func TestMediaStorePath_NamesTheAccountsDatabase(t *testing.T) {
	t.Parallel()

	got := mediaStorePath("/data/signal", "acc1")
	if want := "/data/signal/acc1-media.db"; got != want {
		t.Errorf("mediaStorePath = %q, want %q", got, want)
	}
}

func TestOpenMediaStore_CreatesAPrivateFreshDatabase(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "signal")
	store, err := openMediaStore(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.close() }()

	info, err := os.Stat(mediaStorePath(dir, "acc1"))
	if err != nil {
		t.Fatalf("media store file was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("media store file mode = %o, want 0600", perm)
	}
}

// TestAttachment_PutThenGetRoundTrips checks that an attachment
// pointer's fields survive a save and load, including after the
// store is closed and reopened, standing in for a helper restart.
func TestAttachment_PutThenGetRoundTrips(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "signal")
	store, err := openMediaStore(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatal(err)
	}

	ptr := &signalpb.AttachmentPointer{
		AttachmentIdentifier: &signalpb.AttachmentPointer_CdnId{CdnId: 42},
		Key:                  []byte{1, 2, 3},
		Digest:               []byte{4, 5, 6},
		Size:                 proto.Uint32(123),
		CdnNumber:            proto.Uint32(2),
		ContentType:          proto.String("image/jpeg"),
		FileName:             proto.String("photo.jpg"),
	}
	if err := store.putAttachment(t.Context(), "conv-1", "msg-1", ptr); err != nil {
		t.Fatal(err)
	}
	if err := store.close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := openMediaStore(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.close() }()

	got, found, err := reopened.attachment(t.Context(), "conv-1", "msg-1")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("attachment() reported not found after a reopen")
	}
	if got.GetCdnId() != 42 || string(got.GetKey()) != "\x01\x02\x03" || got.GetSize() != 123 || got.GetContentType() != "image/jpeg" {
		t.Errorf("attachment() = %+v", got)
	}
}

func TestAttachment_UnknownMessageNotFound(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	_, found, err := store.attachment(t.Context(), "conv-1", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("attachment() found a message never saved")
	}
}

// TestMessageMeta_PutThenGetRoundTrips checks that a message's server
// GUID and report-spam token survive a save and load.
func TestMessageMeta_PutThenGetRoundTrips(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	if err := store.putMessageMeta(t.Context(), "conv-1", "msg-1", "guid-1", []byte{9, 9}); err != nil {
		t.Fatal(err)
	}

	guid, token, found, err := store.messageMeta(t.Context(), "conv-1", "msg-1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || guid != "guid-1" || string(token) != "\x09\x09" {
		t.Errorf("messageMeta() = %q %v found=%t", guid, token, found)
	}
}

func TestMessageMeta_UnknownMessageNotFound(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	_, _, found, err := store.messageMeta(t.Context(), "conv-1", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("messageMeta() found a message never saved")
	}
}

// TestPollStore_VoteAndCloseSurviveAReopen checks the same poll
// lifecycle poll_test.go checks against the connector, but against
// the store directly and across a close and reopen, standing in for
// a helper restart.
func TestPollStore_VoteAndCloseSurviveAReopen(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "signal")
	store, err := openMediaStore(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.putPoll(t.Context(), "conv-1", "msg-1", newPoll{question: "Pizza?", options: []string{"Yes", "No"}, multipleChoice: false}); err != nil {
		t.Fatal(err)
	}
	if err := store.setVotes(t.Context(), "conv-1", "msg-1", "self", []int{0}); err != nil {
		t.Fatal(err)
	}
	if err := store.setVotes(t.Context(), "conv-1", "msg-1", "other", []int{0}); err != nil {
		t.Fatal(err)
	}
	if err := store.close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := openMediaStore(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.close() }()

	poll, found, err := reopened.pollTally(t.Context(), "conv-1", "msg-1", "self")
	if err != nil {
		t.Fatal(err)
	}
	if !found || poll.Options[0].Votes != 2 || !poll.Options[0].Chosen || poll.TotalVoters != 2 {
		t.Fatalf("pollTally() = %+v found=%t", poll, found)
	}

	if err := reopened.closePoll(t.Context(), "conv-1", "msg-1"); err != nil {
		t.Fatal(err)
	}
	poll, found, err = reopened.pollTally(t.Context(), "conv-1", "msg-1", "self")
	if err != nil {
		t.Fatal(err)
	}
	if !found || !poll.Closed {
		t.Errorf("pollTally() after close = %+v found=%t, want closed", poll, found)
	}
}

// TestPollStore_RevoteReplaces checks that a second vote from the
// same voter replaces their first, since Signal always sends a
// voter's whole current choice rather than a delta.
func TestPollStore_RevoteReplaces(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	if err := store.putPoll(t.Context(), "conv-1", "msg-1", newPoll{question: "Pizza?", options: []string{"Yes", "No"}, multipleChoice: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.setVotes(t.Context(), "conv-1", "msg-1", "voter", []int{0}); err != nil {
		t.Fatal(err)
	}
	if err := store.setVotes(t.Context(), "conv-1", "msg-1", "voter", []int{1}); err != nil {
		t.Fatal(err)
	}

	poll, found, err := store.pollTally(t.Context(), "conv-1", "msg-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if !found || poll.Options[0].Votes != 0 || poll.Options[1].Votes != 1 {
		t.Errorf("pollTally() after revote = %+v", poll.Options)
	}
}

// TestOpenMediaStoreDSN_InvalidDSN checks that an unopenable data
// source reports an error rather than a usable store.
func TestOpenMediaStoreDSN_InvalidDSN(t *testing.T) {
	t.Parallel()

	dsn := "file:" + filepath.Join(t.TempDir(), "missing.db") + "?mode=ro"
	if _, err := openMediaStoreDSN(t.Context(), dsn); err == nil {
		t.Error("openMediaStoreDSN() succeeded opening a missing database read-only, want an error")
	}
}

// TestOpenMediaStore_UnwritableDirectory checks that a directory this
// account cannot create reports an error rather than a usable store.
func TestOpenMediaStore_UnwritableDirectory(t *testing.T) {
	t.Parallel()

	// A file where a directory needs to be: MkdirAll fails on it.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := openMediaStore(t.Context(), filepath.Join(blocker, "signal"), "acc1"); err == nil {
		t.Error("openMediaStore() succeeded under a file, want an error")
	}
}

// TestMediaStore_ClosedDatabaseErrors checks that every write and read
// reports an error, rather than silently losing or fabricating data,
// once the underlying database is closed.
func TestMediaStore_ClosedDatabaseErrors(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	if err := store.close(); err != nil {
		t.Fatal(err)
	}

	if err := store.putAttachment(t.Context(), "c", "m", &signalpb.AttachmentPointer{}); err == nil {
		t.Error("putAttachment() on a closed store succeeded")
	}
	if _, _, err := store.attachment(t.Context(), "c", "m"); err == nil {
		t.Error("attachment() on a closed store succeeded")
	}
	if err := store.putMessageMeta(t.Context(), "c", "m", "g", nil); err == nil {
		t.Error("putMessageMeta() on a closed store succeeded")
	}
	if _, _, _, err := store.messageMeta(t.Context(), "c", "m"); err == nil {
		t.Error("messageMeta() on a closed store succeeded")
	}
	if err := store.putPoll(t.Context(), "c", "m", newPoll{question: "q", options: []string{"a"}, multipleChoice: false}); err == nil {
		t.Error("putPoll() on a closed store succeeded")
	}
	if err := store.setVotes(t.Context(), "c", "m", "v", []int{0}); err == nil {
		t.Error("setVotes() on a closed store succeeded")
	}
	if err := store.closePoll(t.Context(), "c", "m"); err == nil {
		t.Error("closePoll() on a closed store succeeded")
	}
	if _, _, err := store.pollTally(t.Context(), "c", "m", "v"); err == nil {
		t.Error("pollTally() on a closed store succeeded")
	}
	if _, err := store.pollOptions(t.Context(), "c", "m"); err == nil {
		t.Error("pollOptions() on a closed store succeeded")
	}
	if _, _, err := store.pollVoters(t.Context(), "c", "m"); err == nil {
		t.Error("pollVoters() on a closed store succeeded")
	}
}

func TestPollStore_UnknownMessageNotFound(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	_, found, err := store.pollTally(t.Context(), "conv-1", "missing", "")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("pollTally() found a poll never saved")
	}
}

// TestNextVoteCount_IncrementsPerVoterPerMessage checks that each
// voter's own counter for a poll starts at one and increases with
// every vote, independently of any other voter's or message's own
// counter.
func TestNextVoteCount_IncrementsPerVoterPerMessage(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	first, err := store.nextVoteCount(t.Context(), "conv-1", "msg-1", "voter-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.nextVoteCount(t.Context(), "conv-1", "msg-1", "voter-1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.nextVoteCount(t.Context(), "conv-1", "msg-1", "voter-2")
	if err != nil {
		t.Fatal(err)
	}

	if first != 1 || second != 2 {
		t.Errorf("nextVoteCount() = %d, %d, want 1, 2", first, second)
	}
	if other != 1 {
		t.Errorf("a second voter's nextVoteCount() = %d, want 1", other)
	}
}

// TestRecentIncoming_NewestFirstUpToLimit checks that recentIncoming
// lists a conversation's own saved messages newest first, bounded by
// limit, and never a message saved for a different conversation.
func TestRecentIncoming_NewestFirstUpToLimit(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	for _, id := range []string{"msg-1", "msg-2", "msg-3"} {
		if err := store.putMessageMeta(t.Context(), "conv-1", id, "guid-"+id, []byte("token-"+id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.putMessageMeta(t.Context(), "conv-2", "other", "guid-other", nil); err != nil {
		t.Fatal(err)
	}

	metas, err := store.recentIncoming(t.Context(), "conv-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 || metas[0].messageID != "msg-3" || metas[1].messageID != "msg-2" {
		t.Errorf("recentIncoming() = %+v, want msg-3 then msg-2", metas)
	}
}

// TestRecentIncoming_UnknownConversationEmpty checks that a
// conversation with nothing saved reports no messages rather than an
// error.
func TestRecentIncoming_UnknownConversationEmpty(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	metas, err := store.recentIncoming(t.Context(), "missing", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 0 {
		t.Errorf("recentIncoming() = %+v, want none", metas)
	}
}
