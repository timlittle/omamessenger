package whatsapp

// vote_crypto_test.go checks the actual poll-vote encryption whatsmeow
// implements, with a real client over an in-memory session database,
// rather than the fakeDevice the rest of this file's tests use: it
// proves this connector's own option hashing (see pollOptionID) agrees
// with what a real vote decrypts to, and that a vote cast with a secret
// this run never saw itself still fails to decrypt rather than panicking.

import (
	"database/sql"
	"encoding/hex"
	"testing"

	"go.mau.fi/whatsmeow"
	waAdv "go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "modernc.org/sqlite"
)

// newTestClient returns a real whatsmeow client over a fresh, private,
// in-memory session database, with ownJID as its own identity, so
// EncryptPollVote and DecryptPollVote run their real cryptography
// without reaching WhatsApp's servers. The account details whatsmeow's
// own pairing handshake would normally fill in are faked with
// correctly sized placeholders: PutDevice checks their lengths, not
// their content, and nothing here ever signs in for real.
func newTestClient(t *testing.T, ownJID types.JID) *whatsmeow.Client {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := t.Context()
	container := sqlstore.NewWithDB(db, "sqlite", waLog.Noop)
	if err := container.Upgrade(ctx); err != nil {
		t.Fatal(err)
	}

	dev, err := container.GetFirstDevice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dev.ID = &ownJID
	dev.Account = &waAdv.ADVSignedDeviceIdentity{
		Details: []byte{}, AccountSignature: make([]byte, 64), AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64),
	}
	if err := container.PutDevice(ctx, dev); err != nil {
		t.Fatal(err)
	}

	return whatsmeow.NewClient(dev, waLog.Noop)
}

func TestPollVoteCrypto_RoundTripsWithTheStoredSecret(t *testing.T) {
	// Not t.Parallel(): go.mau.fi/libsignal's own package-level logger
	// lazily initializes itself with no synchronization the first time
	// any signature is calculated, which races under -race when two
	// of these tests build their first real whatsmeow client at once.
	// That is a bug in that dependency, not in this code, and the only
	// two tests in this package that ever touch real whatsmeow crypto
	// are these; running them one at a time avoids it.
	ctx := t.Context()
	self := types.NewJID("15550000000", types.DefaultUserServer)
	poller := types.NewJID("15551234567", types.DefaultUserServer)
	cli := newTestClient(t, self)

	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i)
	}
	if err := cli.Store.MsgSecrets.PutMessageSecret(ctx, poller, poller, "poll1", secret); err != nil {
		t.Fatal(err)
	}

	pollInfo := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: poller, Sender: poller, IsFromMe: false},
		ID:            "poll1",
	}

	voteMsg, err := cli.BuildPollVote(ctx, pollInfo, []string{"Pizza"})
	if err != nil {
		t.Fatal(err)
	}

	evt := &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: poller, Sender: self, IsFromMe: true}},
		Message: voteMsg,
	}

	decrypted, err := cli.DecryptPollVote(ctx, evt)
	if err != nil {
		t.Fatal(err)
	}

	got := decrypted.GetSelectedOptions()
	want := whatsmeow.HashPollOptions([]string{"Pizza"})
	if len(got) != 1 || string(got[0]) != string(want[0]) {
		t.Errorf("decrypted hash = %x, want %x", got, want)
	}

	// This connector's own id for the option it saved from the poll's
	// creation message must be the hex of exactly this hash, or a vote
	// decrypted this way could never be matched back to it.
	if id := pollOptionID("Pizza"); id != hex.EncodeToString(got[0]) {
		t.Errorf("pollOptionID(%q) = %q, want the decrypted hash %x", "Pizza", id, got[0])
	}
}

func TestPollVoteCrypto_FailsWithoutTheStoredSecret(t *testing.T) {
	// Not t.Parallel(); see the sibling test above.
	ctx := t.Context()
	self := types.NewJID("15550000000", types.DefaultUserServer)
	poller := types.NewJID("15551234567", types.DefaultUserServer)
	cli := newTestClient(t, self)

	// No secret was ever stored for this poll, the way it never would be
	// if this connector's run started after the poll's creation message
	// was already gone from the service.
	pollInfo := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: poller, Sender: poller, IsFromMe: false},
		ID:            "poll1",
	}

	if _, err := cli.BuildPollVote(ctx, pollInfo, []string{"Pizza"}); err == nil {
		t.Error("BuildPollVote = nil error, want one for a poll with no stored secret")
	}
}
