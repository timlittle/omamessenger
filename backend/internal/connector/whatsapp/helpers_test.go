package whatsapp

// Pairing fakes shared by this package's tests. They stand in for a real
// whatsmeow client so Run and the pairing sequence can be driven and
// checked without ever reaching WhatsApp's servers.

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// fakeDevice is a hand-written double for device: a test drives it by
// pushing items onto its QR channel and calling status, and reads back
// what the connector called on it.
type fakeDevice struct {
	mu sync.Mutex

	paired     bool
	connectErr error
	qrErr      error
	pairCode   string
	pairErr    error

	connects     int
	disconnects  int
	closed       bool
	loggedOut    bool
	pairedPhones []string
	handler      func(status string)
	unregistered bool

	codes chan whatsmeow.QRChannelItem

	// sendErr, sendBlocks, markReadErr and nextMessageID script the send
	// and read methods send_test.go and receipts_test.go drive; sent and
	// markReadCalls record what Send and MarkRead actually asked for.
	sendErr       error
	sendBlocks    bool
	nextMessageID types.MessageID
	sent          []sentCall
	markReadErr   error
	markReadCalls []markReadCall
	eventHandler  func(evt any)

	// duringConnect holds events connect delivers before it returns, the
	// way WhatsApp sends history and offline messages as soon as a
	// session connects.
	duringConnect []any

	// groupNames, groupMembers, groupErr and groupCalls script and record
	// groupInfo, which history.go and live.go call to resolve a group's
	// name and member count.
	groupNames   map[string]string
	groupMembers map[string]int
	groupErr     error
	groupCalls   []types.JID

	// participants and participantsErr script groupParticipants, keyed
	// by the string form of the group JID.
	participants    map[string][]types.GroupParticipant
	participantsErr error

	// contactNames scripts contactName, keyed by the string form of
	// whichever JID (a phone JID or a LID) the lookup should resolve.
	contactNames map[string]string

	// lidPhones scripts altJID, keyed by the string form of the LID a
	// test wants resolved to a phone JID; altJID also resolves the
	// reverse direction automatically from the same entry, the way
	// whatsmeow's own LID store learns both directions from one
	// mapping. A JID with no entry on either side resolves to an empty
	// JID, the way an unmapped one really does.
	lidPhones map[string]types.JID

	// selfJID scripts isSelfChat and selfChatID: a JID whose bare form
	// matches it, or matches selfLID, is this account's own self-chat,
	// and selfChatID always returns selfJID's remote id. The zero value
	// matches nothing.
	selfJID types.JID

	// selfLID additionally scripts isSelfChat, for a test that addresses
	// the self-chat by a LID distinct from selfJID, the way WhatsApp's
	// own servers increasingly do; selfChatID still canonicalizes to
	// selfJID either way. The zero value matches nothing beyond selfJID.
	selfLID types.JID

	// downloadData, downloadErr and downloadBlocks script downloadMedia,
	// and downloadCalls records what it was asked to fetch; fetch_test.go
	// drives these. downloadErrPaths, keyed by a reference's DirectPath,
	// fails only the attempt for that one path rather than every
	// attempt, the way a real stale link fails while a fresh one from a
	// media retry succeeds; retry_test.go drives this one.
	downloadData     []byte
	downloadErr      error
	downloadErrPaths map[string]error
	downloadBlocks   bool
	downloadCalls    []mediaRef

	// uploadResp and uploadErr script uploadMedia, and uploadCalls
	// records what it was asked to upload; upload_test.go drives these.
	uploadResp  whatsmeow.UploadResponse
	uploadErr   error
	uploadCalls []uploadCall

	// appStateErr and appStateBlocks script sendAppState, which
	// organize_test.go drives to check pin and archive changes without
	// reaching WhatsApp's servers; appStatePatches records what was
	// sent. settings is this fake's chat settings store, mirroring
	// whatsmeow's own (store.ChatSettingsStore): a successful
	// sendAppState applies its patch's mutations to it synchronously,
	// exactly as a real SendAppState updates its store before
	// returning (see device.go and docs/decisions.md), and a test may
	// also seed it directly to stand in for a chat WhatsApp already
	// had pinned or archived from before this connector ever saw it.
	// sendAppState itself, along with applyAppState, chatSettings and
	// setChatSettings, is defined in organize_test.go, beside the pin
	// and archive tests that are the reason all four exist at all.
	appStateErr     error
	appStateBlocks  bool
	appStatePatches []appstate.PatchInfo
	settings        map[string]chatSettingsEntry

	// mediaRetryErr scripts sendMediaRetryReceipt, and mediaRetryCalls
	// records what it was asked to send; retry_test.go drives these.
	mediaRetryErr   error
	mediaRetryCalls []mediaRetryCall

	// decryptVoteResp and decryptVoteErr script decryptPollVote;
	// buildVoteResp and buildVoteErr script buildPollVote, and
	// buildVoteCalls records what it was asked to build; vote_test.go
	// drives these.
	decryptVoteResp *waE2E.PollVoteMessage
	decryptVoteErr  error
	buildVoteResp   *waE2E.Message
	buildVoteErr    error
	buildVoteCalls  []buildVoteCall

	// requestHistoryErr scripts requestOlderHistory, and
	// requestHistoryCalls records what it was asked to request;
	// history_ondemand_test.go drives these.
	requestHistoryErr   error
	requestHistoryCalls []requestHistoryCall
}

// buildVoteCall records one call to buildPollVote.
type buildVoteCall struct {
	pollInfo    *types.MessageInfo
	optionNames []string
}

// requestHistoryCall records one call to requestOlderHistory.
type requestHistoryCall struct {
	anchor *types.MessageInfo
	count  int
}

// mediaRetryCall records one call to sendMediaRetryReceipt.
type mediaRetryCall struct {
	info     *types.MessageInfo
	mediaKey []byte
}

// uploadCall records one call to uploadMedia.
type uploadCall struct {
	data []byte
	kind mediaKind
}

// sentCall records one call to sendMessage.
type sentCall struct {
	jid types.JID
	msg *waE2E.Message
	id  types.MessageID
}

// markReadCall records one call to markRead.
type markReadCall struct {
	ids    []types.MessageID
	chat   types.JID
	sender types.JID
}

// newFakeDevice returns a fake with an open QR channel and no session.
func newFakeDevice() *fakeDevice {
	return &fakeDevice{codes: make(chan whatsmeow.QRChannelItem, 8)}
}

// connect records the call and reports connectErr. An already-paired
// device simulates whatsmeow firing its Connected event right away, as
// a reconnect to an existing session does; a device still pairing does
// not, since real pairing's handshake only finishes well after Connect
// returns, so a test fires that event itself once it sends success.
func (d *fakeDevice) connect(context.Context) error {
	d.mu.Lock()
	d.connects++
	err, handler, paired, during := d.connectErr, d.handler, d.paired, d.duringConnect
	d.mu.Unlock()

	if err == nil && paired && handler != nil {
		handler(statusConnected)
	}
	for _, evt := range during {
		d.fireEvent(evt)
	}

	return err
}

// disconnect records the call.
func (d *fakeDevice) disconnect() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.disconnects++
}

// close records the call.
func (d *fakeDevice) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.closed = true

	return nil
}

// isPaired reports the session a test set up.
func (d *fakeDevice) isPaired() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.paired
}

// logOut records the call.
func (d *fakeDevice) logOut(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.loggedOut = true

	return nil
}

// qrCodes returns the fake's scripted channel, or qrErr.
func (d *fakeDevice) qrCodes(context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	if d.qrErr != nil {
		return nil, d.qrErr
	}

	return d.codes, nil
}

// pairPhone records number and reports the scripted code or error.
func (d *fakeDevice) pairPhone(_ context.Context, number string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pairedPhones = append(d.pairedPhones, number)
	if d.pairErr != nil {
		return "", d.pairErr
	}

	return d.pairCode, nil
}

// onStatus records the handler and returns an unregister func that marks
// itself called.
func (d *fakeDevice) onStatus(handler func(status string)) func() {
	d.mu.Lock()
	d.handler = handler
	d.mu.Unlock()

	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		d.unregistered = true
	}
}

// status invokes the registered handler, simulating a whatsmeow event.
func (d *fakeDevice) status(s string) {
	d.mu.Lock()
	h := d.handler
	d.mu.Unlock()

	if h != nil {
		h(s)
	}
}

// sendMessage records the call and reports sendErr, or blocks on ctx
// when sendBlocks is set, as a real send that never hears back from the
// server does.
func (d *fakeDevice) sendMessage(ctx context.Context, jid types.JID, msg *waE2E.Message, id types.MessageID) (whatsmeow.SendResponse, error) {
	d.mu.Lock()
	d.sent = append(d.sent, sentCall{jid: jid, msg: msg, id: id})
	blocks, err := d.sendBlocks, d.sendErr
	d.mu.Unlock()

	if blocks {
		<-ctx.Done()
		return whatsmeow.SendResponse{}, ctx.Err()
	}
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	return whatsmeow.SendResponse{ID: id}, nil
}

// generateMessageID returns the scripted id, or a fixed one when the
// test does not care which.
func (d *fakeDevice) generateMessageID() types.MessageID {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.nextMessageID != "" {
		return d.nextMessageID
	}

	return "generated-id"
}

// markRead records the call and reports markReadErr.
func (d *fakeDevice) markRead(_ context.Context, ids []types.MessageID, chat, sender types.JID) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.markReadCalls = append(d.markReadCalls, markReadCall{ids: ids, chat: chat, sender: sender})

	return d.markReadErr
}

// onEvent records the handler and returns an unregister func that clears
// it.
func (d *fakeDevice) onEvent(handler func(evt any)) func() {
	d.mu.Lock()
	d.eventHandler = handler
	d.mu.Unlock()

	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		d.eventHandler = nil
	}
}

// fireEvent invokes the registered event handler, simulating whatsmeow
// dispatching evt.
func (d *fakeDevice) fireEvent(evt any) {
	d.mu.Lock()
	h := d.eventHandler
	d.mu.Unlock()

	if h != nil {
		h(evt)
	}
}

// groupInfo records the call and reports the scripted name, member
// count or error.
func (d *fakeDevice) groupInfo(_ context.Context, jid types.JID) (string, int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.groupCalls = append(d.groupCalls, jid)
	if d.groupErr != nil {
		return "", 0, d.groupErr
	}

	return d.groupNames[jid.String()], d.groupMembers[jid.String()], nil
}

// groupParticipants reports the scripted participants or error.
func (d *fakeDevice) groupParticipants(_ context.Context, jid types.JID) ([]types.GroupParticipant, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.participantsErr != nil {
		return nil, d.participantsErr
	}

	return d.participants[jid.String()], nil
}

// contactName reports the scripted name for jid, mapping a LID to its
// scripted phone JID first when jid itself has no name of its own, the
// same priority order the real device's contactName documents; it
// reports "" when the test did not script either.
func (d *fakeDevice) contactName(_ context.Context, jid types.JID) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if name := d.contactNames[jid.String()]; name != "" {
		return name
	}

	if phone, ok := d.lidPhones[jid.String()]; ok {
		return d.contactNames[phone.String()]
	}

	return ""
}

// isSelfChat reports whether jid's bare form matches the scripted
// selfJID or selfLID, the way the real device compares against the
// account's own phone JID or LID.
func (d *fakeDevice) isSelfChat(_ context.Context, jid types.JID) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	bare := jid.ToNonAD()

	return !d.selfJID.IsEmpty() && bare == d.selfJID.ToNonAD() ||
		!d.selfLID.IsEmpty() && bare == d.selfLID.ToNonAD()
}

// selfChatID returns the scripted selfJID's remote id, the way the real
// device always canonicalizes the self-chat to its phone JID.
func (d *fakeDevice) selfChatID() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return remoteID(d.selfJID)
}

// altJID returns the scripted other address form for jid, the way the
// real device maps a LID to a phone JID, or a phone JID to a LID,
// through whatsmeow's local LID store: jid's own entry in lidPhones
// when it is itself a LID, or, when it is a phone JID, whichever LID
// lidPhones maps to it.
func (d *fakeDevice) altJID(_ context.Context, jid types.JID) types.JID {
	d.mu.Lock()
	defer d.mu.Unlock()

	if jid.Server == types.HiddenUserServer {
		return d.lidPhones[jid.String()]
	}

	for lidStr, phone := range d.lidPhones {
		if phone.String() != jid.String() {
			continue
		}
		if lid, err := types.ParseJID(lidStr); err == nil {
			return lid
		}
	}

	return types.JID{}
}

// downloadMedia records ref and reports the scripted bytes or error, or
// blocks on ctx when downloadBlocks is set, as a real download that
// never hears back from WhatsApp's media servers does. A path-specific
// error in downloadErrPaths overrides the plain downloadErr for that
// one reference only.
func (d *fakeDevice) downloadMedia(ctx context.Context, ref mediaRef) ([]byte, error) {
	d.mu.Lock()
	d.downloadCalls = append(d.downloadCalls, ref)
	blocks, data, err := d.downloadBlocks, d.downloadData, d.downloadErr
	if pathErr, ok := d.downloadErrPaths[ref.DirectPath]; ok {
		err = pathErr
	}
	d.mu.Unlock()

	if blocks {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err != nil {
		// Real whatsmeow still returns the decrypted bytes alongside
		// whatsmeow.ErrInvalidMediaSHA256: its own downloadAndDecrypt
		// sets data before running that specific check. A test that
		// wants to drive recoverStaleDigest sets downloadData as well
		// as downloadErr to mimic that; every other scripted error
		// means no data at all, matching every failure that happens
		// before whatsmeow ever gets to decrypt anything.
		return data, err
	}

	return data, nil
}

// uploadMedia records the call and reports the scripted response or
// error.
func (d *fakeDevice) uploadMedia(_ context.Context, data []byte, kind mediaKind) (whatsmeow.UploadResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.uploadCalls = append(d.uploadCalls, uploadCall{data: data, kind: kind})
	if d.uploadErr != nil {
		return whatsmeow.UploadResponse{}, d.uploadErr
	}

	return d.uploadResp, nil
}

// sendMediaRetryReceipt records the call and reports mediaRetryErr.
func (d *fakeDevice) sendMediaRetryReceipt(_ context.Context, info *types.MessageInfo, mediaKey []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.mediaRetryCalls = append(d.mediaRetryCalls, mediaRetryCall{info: info, mediaKey: mediaKey})

	return d.mediaRetryErr
}

// requestOlderHistory records the call and reports requestHistoryErr.
func (d *fakeDevice) requestOlderHistory(_ context.Context, anchor *types.MessageInfo, count int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.requestHistoryCalls = append(d.requestHistoryCalls, requestHistoryCall{anchor: anchor, count: count})

	return d.requestHistoryErr
}

// decryptPollVote reports the scripted vote or error.
func (d *fakeDevice) decryptPollVote(_ context.Context, _ *events.Message) (*waE2E.PollVoteMessage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.decryptVoteResp, d.decryptVoteErr
}

// buildPollVote records the call and reports the scripted message or
// error.
func (d *fakeDevice) buildPollVote(_ context.Context, pollInfo *types.MessageInfo, optionNames []string) (*waE2E.Message, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.buildVoteCalls = append(d.buildVoteCalls, buildVoteCall{pollInfo: pollInfo, optionNames: optionNames})
	if d.buildVoteErr != nil {
		return nil, d.buildVoteErr
	}

	return d.buildVoteResp, nil
}

// connectedTo returns a connector whose Send, MarkRead and event
// handling act on dev, as if Run had already connected it. It carries
// no media store: a test that needs React or a reply quoted with more
// than a stanza id uses connectedToWithMedia instead.
func connectedTo(dev device, sink connector.Sink) *Connector {
	c := &Connector{account: domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp}, answers: make(chan answer, 1)}
	c.connected(dev, sink, nil)

	return c
}

// connectedToWithMedia is connectedTo with an in-memory media store
// wired in too, for a test that reacts to, replies to or fetches a
// message's attachment and so needs somewhere to save and look up
// message keys or media references; fetch it back with c.mediaFor() to
// seed a reference before calling FetchMedia, or to read one back after
// Send.
func connectedToWithMedia(t *testing.T, dev device, sink connector.Sink) *Connector {
	t.Helper()

	media, err := newInMemoryMediaStore(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = media.close() }) // a synctest caller that must close it sooner does so itself

	c := &Connector{account: domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp}, answers: make(chan answer, 1)}
	c.connected(dev, sink, media)

	return c
}

// newConnectorWithMedia returns a connector whose Send, MarkRead and
// event handling act on dev and media, as if Run had already connected
// it, without creating a media store of its own the way connectedTo and
// connectedToWithMedia do. A test that wants to simulate a restart —
// this run's own in-memory state starting empty, but a media store an
// earlier run already wrote message keys into still on disk — opens
// that media store once with newTestMediaStore and builds a connector
// around it with this, for however many simulated "runs" the test
// needs.
func newConnectorWithMedia(dev device, sink connector.Sink, media *mediaStore) *Connector {
	c := &Connector{account: domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp}, answers: make(chan answer, 1)}
	c.connected(dev, sink, media)

	return c
}

// connectedFixture returns a fake device, a recording sink and a
// connector wired to them exactly as connectedTo does, for the common
// case of a test that drives Connector's exported methods, or an
// unexported event handler, without needing a media store.
func connectedFixture(t *testing.T) (*fakeDevice, *connectortest.Sink, *Connector) {
	t.Helper()
	dev, sink := newFakeDevice(), &connectortest.Sink{}

	return dev, sink, connectedTo(dev, sink)
}

// connectedMediaFixture is connectedFixture with an in-memory media
// store wired in too, exactly as connectedToWithMedia does, returning
// the store so a test can seed or read back a message key or media
// reference with c.mediaFor().
func connectedMediaFixture(t *testing.T) (*fakeDevice, *connectortest.Sink, *Connector, *mediaStore) {
	t.Helper()
	dev, sink := newFakeDevice(), &connectortest.Sink{}
	c := connectedToWithMedia(t, dev, sink)

	return dev, sink, c, c.mediaFor()
}

// handlerFixture returns a connector, a fake device and a recording sink
// for a test that calls one of the package's unexported event handlers
// directly, the way history_test.go drives handleHistorySync and
// live_test.go drives live.go's handlers, without a media store.
func handlerFixture(t *testing.T) (*Connector, *fakeDevice, *connectortest.Sink) {
	t.Helper()

	return New(domain.Account{ID: "wa"}, t.TempDir()), newFakeDevice(), &connectortest.Sink{}
}

// handlerMediaFixture is handlerFixture with a temporary on-disk media
// store wired in too, for a handler test that saves or looks up a
// message key or media reference.
func handlerMediaFixture(t *testing.T) (*Connector, *fakeDevice, *connectortest.Sink, *mediaStore) {
	t.Helper()

	return New(domain.Account{ID: "wa"}, t.TempDir()), newFakeDevice(), &connectortest.Sink{}, newTestMediaStore(t)
}

// newTestMediaStore opens a media store in a fresh temporary directory,
// closing it when the test ends.
func newTestMediaStore(t *testing.T) *mediaStore {
	t.Helper()

	store, err := openMediaStore(t.Context(), filepath.Join(t.TempDir(), "whatsapp"), "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.close() })

	return store
}

// newInMemoryMediaStore opens a private, in-memory media store, for a
// connector under test that never reads a reference back out of this
// package; a test that does should open its own with newTestMediaStore.
func newInMemoryMediaStore(ctx context.Context) (*mediaStore, error) {
	return openMediaStoreDSN(ctx, "file::memory:")
}

// strPtr takes the address of a string literal, since the generated
// protobuf structs this package normalizes hold every optional field as
// a pointer.
func strPtr(s string) *string { return &s }

// boolPtr takes the address of a bool literal, for the same reason.
func boolPtr(b bool) *bool { return &b }

// u32 takes the address of a uint32 literal, for the same reason.
func u32(n uint32) *uint32 { return &n }

// u64 takes the address of a uint64 literal, for the same reason.
func u64(n uint64) *uint64 { return &n }

// senderNameCall records one call to recordingSink.SenderName.
type senderNameCall struct {
	accountID, senderRemoteID, name string
}

// recordingSink wraps connectortest.Sink, additionally implementing
// connector.SenderNamer, so a test can confirm a resolved name also
// corrects already-stored messages (see contacts.go's
// retitleDirectChat), which connectortest.Sink alone has no need to
// support since most of this package's tests check only what was
// reported, not what a real Ingest would do with it.
type recordingSink struct {
	*connectortest.Sink
	senderNames []senderNameCall
}

// SenderName records the call.
func (s *recordingSink) SenderName(_ context.Context, accountID, senderRemoteID, name string) {
	s.senderNames = append(s.senderNames, senderNameCall{accountID, senderRemoteID, name})
}

// contains reports whether s holds substr.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
