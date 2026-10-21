package connector_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// onePixelPNG returns a tiny, real PNG, for a test that needs bytes
// imagesafe actually accepts rather than an arbitrary placeholder.
func onePixelPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)

	return buf.Bytes()
}

// onePixelGIF returns a tiny, real GIF, for a test that needs bytes a
// genuine GIF header check and imagesafe's own sanitizer both actually
// accept, rather than an arbitrary placeholder.
func onePixelGIF() []byte {
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.RGBA{R: 200, G: 100, B: 50, A: 255}})
	img.SetColorIndex(0, 0, 0)

	var buf bytes.Buffer
	_ = gif.Encode(&buf, img, nil)

	return buf.Bytes()
}

// onePlayableMP4 returns a minimal, well-formed MP4 file an ordinary
// H.264 video at 640x480, for a test that needs bytes videosafe
// actually accepts rather than an arbitrary placeholder: one moov box
// with an mvhd and a single video trak, just enough of a real file's
// own box structure for videosafe's own parser to read.
func onePlayableMP4() []byte {
	mediaHeader := func(timescale, duration uint32) []byte {
		buf := make([]byte, 20)
		binary.BigEndian.PutUint32(buf[12:16], timescale)
		binary.BigEndian.PutUint32(buf[16:20], duration)
		return buf
	}
	box := func(boxType string, body []byte) []byte {
		buf := make([]byte, 8+len(body))
		binary.BigEndian.PutUint32(buf[0:4], uint32(len(buf)))
		copy(buf[4:8], boxType)
		copy(buf[8:], body)
		return buf
	}

	sampleDescription := make([]byte, 8+44)
	binary.BigEndian.PutUint32(sampleDescription[4:8], 1) // entry_count
	binary.BigEndian.PutUint32(sampleDescription[8:12], 44)
	copy(sampleDescription[12:16], "avc1")
	binary.BigEndian.PutUint16(sampleDescription[40:42], 640)
	binary.BigEndian.PutUint16(sampleDescription[42:44], 480)

	mdhd := box("mdhd", mediaHeader(1000, 5000))
	hdlr := box("hdlr", append(make([]byte, 8), []byte("vide")...))
	stsd := box("stsd", sampleDescription)
	stsz := box("stsz", make([]byte, 12))
	stbl := box("stbl", append(append([]byte{}, stsd...), stsz...))
	minf := box("minf", stbl)
	mdia := box("mdia", append(append(append([]byte{}, mdhd...), hdlr...), minf...))
	trak := box("trak", mdia)
	mvhd := box("mvhd", mediaHeader(1000, 5000))
	moov := box("moov", append(append([]byte{}, mvhd...), trak...))

	return moov
}

// fakeConnector runs the run function, or blocks until cancelled when it
// is nil, and records what it was asked to send.
type fakeConnector struct {
	id   string
	run  func(ctx context.Context) error
	sent []string
}

func (c *fakeConnector) Account() domain.Account {
	return domain.Account{ID: c.id, Service: domain.ServiceWhatsApp, Name: c.id}
}

func (c *fakeConnector) Run(ctx context.Context, _ connector.Sink) error {
	if c.run != nil {
		return c.run(ctx)
	}

	<-ctx.Done()

	return nil
}

func (c *fakeConnector) Send(_ context.Context, _ domain.Conversation, m domain.Message) error {
	c.sent = append(c.sent, m.ID)

	return nil
}

func (c *fakeConnector) MarkRead(context.Context, domain.Conversation) error {
	return nil
}

// signingIn is a connector that signs in, recording what it was given.
type signingIn struct {
	fakeConnector
	answers []string
}

func (c *signingIn) SubmitAuth(_ context.Context, step, value string) error {
	c.answers = append(c.answers, step+"="+value)

	return nil
}

// withHistory is a connector that loads older history, recording where
// from, and says it found count messages.
type withHistory struct {
	fakeConnector
	from  []string
	count int
}

func (c *withHistory) LoadOlder(_ context.Context, _ domain.Conversation, beforeRemoteID string, _ int) (int, error) {
	c.from = append(c.from, beforeRemoteID)

	return c.count, nil
}

// withMedia is a connector that downloads media, recording which message
// and where to.
type withMedia struct {
	fakeConnector
	fetched []string
}

func (c *withMedia) FetchMedia(_ context.Context, _ domain.Conversation, messageRemoteID, path string, _ bool) error {
	c.fetched = append(c.fetched, messageRemoteID+" "+path)

	return nil
}

// withRefresh is a connector that re-reports messages, recording which
// ones it was asked for.
type withRefresh struct {
	fakeConnector
	asked [][]string
}

func (c *withRefresh) RefreshMessages(_ context.Context, _ domain.Conversation, remoteIDs []string) error {
	c.asked = append(c.asked, remoteIDs)

	return nil
}

// withOrganizer is a connector that pins and archives, recording what it
// was asked.
type withOrganizer struct {
	fakeConnector
	pinned   []bool
	archived []bool
}

func (c *withOrganizer) SetPinned(_ context.Context, _ domain.Conversation, pinned bool) error {
	c.pinned = append(c.pinned, pinned)

	return nil
}

func (c *withOrganizer) SetArchived(_ context.Context, _ domain.Conversation, archived bool) error {
	c.archived = append(c.archived, archived)

	return nil
}

// withReactor is a connector that reacts to messages, recording what it
// was asked to set.
type withReactor struct {
	fakeConnector
	reacted []string
}

func (c *withReactor) React(_ context.Context, _ domain.Conversation, messageRemoteID, emoji string) error {
	c.reacted = append(c.reacted, messageRemoteID+" "+emoji)

	return nil
}

// withVoter is a connector that casts votes in polls, recording what it
// was asked to vote for.
type withVoter struct {
	fakeConnector
	voted []string
}

func (c *withVoter) Vote(_ context.Context, _ domain.Conversation, messageRemoteID string, optionIDs []string) error {
	c.voted = append(c.voted, fmt.Sprintf("%s %v", messageRemoteID, optionIDs))

	return nil
}

// withDeleter is a connector that deletes messages, recording what it
// was asked to delete.
type withDeleter struct {
	fakeConnector
	deleted []string
}

func (c *withDeleter) DeleteMessages(_ context.Context, _ domain.Conversation, ids []string, forEveryone bool) error {
	c.deleted = append(c.deleted, fmt.Sprintf("%v %t", ids, forEveryone))

	return nil
}

// withEditor is a connector that edits sent messages, recording what it
// was asked to change.
type withEditor struct {
	fakeConnector
	edited []string
}

func (c *withEditor) EditMessage(_ context.Context, _ domain.Conversation, messageRemoteID, newText string, mentions []domain.Mention) error {
	c.edited = append(c.edited, fmt.Sprintf("%s %q %v", messageRemoteID, newText, mentions))

	return nil
}

// withBlocker is a connector that blocks and unblocks contacts,
// recording what it was asked to set.
type withBlocker struct {
	fakeConnector
	blocked []bool
}

func (c *withBlocker) SetBlocked(_ context.Context, _ domain.Conversation, blocked bool) error {
	c.blocked = append(c.blocked, blocked)

	return nil
}

// withSpamReporter is a connector that reports conversations as spam,
// recording how many times it was asked.
type withSpamReporter struct {
	fakeConnector
	reported int
}

func (c *withSpamReporter) ReportSpam(_ context.Context, _ domain.Conversation) error {
	c.reported++

	return nil
}

// withLogout is a connector that can unlink itself from its service,
// recording whether it was asked to and reporting logoutErr.
type withLogout struct {
	fakeConnector
	loggedOut bool
	logoutErr error
}

func (c *withLogout) Logout(context.Context) error {
	c.loggedOut = true

	return c.logoutErr
}

// accountList records upserted account ids, failing when err is set.
type accountList struct {
	ids []string
	err error
}

func (a *accountList) UpsertAccount(_ context.Context, account domain.Account) error {
	a.ids = append(a.ids, account.ID)

	return a.err
}

// errBroken is the failure fake connectors return.
var errBroken = errors.New("broken")

// startManager starts connectors with a recording sink and stops them when the
// test ends.
func startManager(t *testing.T, ctx context.Context, connectors ...connector.Connector) (*connector.Manager, *connectortest.Sink) {
	t.Helper()

	m, err := connector.NewManager(connectors...)
	if err != nil {
		t.Fatal(err)
	}

	sink := &connectortest.Sink{}
	if err := m.Start(ctx, &accountList{}, sink); err != nil {
		t.Fatal(err)
	}

	return m, sink
}
