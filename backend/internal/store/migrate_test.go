// These tests use package store, not store_test, because a failing
// migration cannot be produced through the public API.
package store

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
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
	steps := []migrationStep{{sql: `CREATE TABLE good(id INTEGER)`}, {sql: `CREATE TABLE broken(`}}

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
	// The GIF migration is fourteenth-from-last, before the audio
	// migration, the pin_order column, the scheduled_messages table,
	// the notification_sound column, the window-open column, the
	// pin_order backfill, the before-archive pin columns, the legacy
	// thumbnail repair, the reply sender id backfill, the sender/title
	// verified columns, the conversation phone column, the
	// verified-by-default reset and the contact verified and phone
	// columns, all appended after it; stop short of all fourteen so
	// inserting the pre-fix rows below still finds them unreclassified.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-14]); err != nil {
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
	if _, err := db.ExecContext(t.Context(), migrations[len(migrations)-14].sql); err != nil {
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
	// The audio migration is thirteenth-from-last, before the pin_order
	// column, the scheduled_messages table, the notification_sound
	// column, the window-open column, the pin_order backfill, the
	// before-archive pin columns, the legacy thumbnail repair, the
	// reply sender id backfill, the sender/title verified columns, the
	// conversation phone column, the verified-by-default reset and the
	// contact verified and phone columns, all appended after it; stop
	// short of all thirteen.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-13]); err != nil {
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
	if _, err := db.ExecContext(t.Context(), migrations[len(migrations)-13].sql); err != nil {
		t.Fatal(err)
	}

	audioAgain := readRawMedia(t, db, "m1")
	if audioAgain != audio {
		t.Errorf("re-running the migration changed an already-reclassified row: %+v, want %+v", audioAgain, audio)
	}
}

// TestMigrate_BackfillsDistinctPinOrderForConversationsPinnedBeforeOrdering
// confirms the migration added after pin_order's own column gives every
// already-pinned conversation a distinct order, by the recency order
// they already displayed in. Before this, a conversation pinned by an
// older build carried the column's bare default, 0, same as every
// other conversation pinned back then: MovePin's swap had nothing to
// swap, since exchanging 0 for 0 leaves both at 0, so the first real
// move a user with old pins made did nothing at all.
func TestMigrate_BackfillsDistinctPinOrderForConversationsPinnedBeforeOrdering(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "pin-backfill.db"))
	// Stop eight short of the full set: the backfill is
	// eighth-from-last, appended after pin_order's own column,
	// scheduled messages, the notification sound column and the
	// window-open column, and ahead of the before-archive pin columns,
	// the legacy thumbnail repair, the reply sender id backfill, the
	// sender/title verified columns, the conversation phone column, the
	// verified-by-default reset and the contact verified and phone
	// columns, all appended after it.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-8]); err != nil {
		t.Fatal(err)
	}

	insertRawAccount(t, db, "wa")
	insertRawConversation(t, db, rawConversation{id: "older", accountID: "wa", pinned: true, lastActivity: 1000})
	insertRawConversation(t, db, rawConversation{id: "newer", accountID: "wa", pinned: true, lastActivity: 2000})
	insertRawConversation(t, db, rawConversation{id: "unpinned", accountID: "wa", pinned: false, lastActivity: 3000})

	if err := migrate(t.Context(), db, migrations); err != nil {
		t.Fatal(err)
	}

	olderOrder := readRawPinOrder(t, db, "older")
	newerOrder := readRawPinOrder(t, db, "newer")
	unpinnedOrder := readRawPinOrder(t, db, "unpinned")

	if olderOrder == newerOrder {
		t.Fatalf("two conversations pinned before ordering existed still share pin_order %d", olderOrder)
	}
	if newerOrder >= olderOrder {
		t.Errorf("newer pin_order = %d, older = %d; want the more recently active one to sort first", newerOrder, olderOrder)
	}
	if unpinnedOrder != 0 {
		t.Errorf("an unpinned conversation's pin_order = %d, want 0, untouched", unpinnedOrder)
	}

	// Re-running the backfill must not reshuffle an already-distinct
	// order: it is idempotent, like the GIF and audio reclassifications
	// above, since recomputing the same recency ranking over rows whose
	// relative order has not changed yields the same ranks.
	if _, err := db.ExecContext(t.Context(), migrations[len(migrations)-8].sql); err != nil {
		t.Fatal(err)
	}

	if got := readRawPinOrder(t, db, "older"); got != olderOrder {
		t.Errorf("re-running the backfill changed older's pin_order: %d, want %d", got, olderOrder)
	}
	if got := readRawPinOrder(t, db, "newer"); got != newerOrder {
		t.Errorf("re-running the backfill changed newer's pin_order: %d, want %d", got, newerOrder)
	}
}

// TestMigrate_SanitizesLegacyThumbnails confirms the migration that
// repairs a thumbnail stored before imagesafe.Sanitize existed: one
// not valid base64 at all, and one whose declared dimensions are far
// over imagesafe.Thumbnail's own bound - the decompression-bomb shape
// this exists to catch - are both cleared to "", while one that merely
// carries a chunk the sanitizer strips is kept, re-encoded to a
// smaller, still-valid image of the same dimensions, so a normal
// user's existing preview is not lost just for upgrading (see
// docs/decisions.md).
func TestMigrate_SanitizesLegacyThumbnails(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "legacy-thumbs.db"))
	// The thumbnail repair is sixth-from-last, appended after the
	// pin_order backfill above and before the reply sender id backfill,
	// the sender/title verified columns, the conversation phone column,
	// the verified-by-default reset and the contact verified and phone
	// columns below.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-6]); err != nil {
		t.Fatal(err)
	}

	clean := fakePNGThumb(8, 8)
	stripped := fakePNGThumbWithChunk(8, 8, "zTXt", bytes.Repeat([]byte{'a'}, 200))
	oversized := fakePNGThumb(2000, 10)
	const invalid = "not valid base64 at all!!"

	insertRawMessage(t, db, "clean", `{"kind":"sticker","thumb":"`+clean+`"}`)
	insertRawMessage(t, db, "stripped", `{"kind":"sticker","thumb":"`+stripped+`"}`)
	insertRawMessage(t, db, "oversized", `{"kind":"sticker","thumb":"`+oversized+`"}`)
	insertRawMessage(t, db, "invalid", `{"kind":"sticker","thumb":"`+invalid+`"}`)
	insertRawMessage(t, db, "none", `{"kind":"file","fileName":"a.txt"}`)

	if err := migrate(t.Context(), db, migrations); err != nil {
		t.Fatal(err)
	}

	// Even an already-well-formed thumbnail is re-encoded now, not left
	// byte-for-byte unchanged (see imagesafe.Reencode and
	// docs/decisions.md), so this only checks it still decodes to the
	// same size, the same as the stripped case below.
	cleanGot := readRawMedia(t, db, "clean").Thumb
	if cleanGot == "" || cleanGot == clean {
		t.Errorf("clean thumbnail = %q, want re-encoded, neither empty nor unchanged", cleanGot)
	}
	requireDecodesTo(t, cleanGot, 8, 8)

	strippedGot := readRawMedia(t, db, "stripped").Thumb
	if strippedGot == "" || strippedGot == stripped {
		t.Fatalf("stripped thumbnail = %q, want re-encoded, neither empty nor unchanged", strippedGot)
	}
	requireDecodesTo(t, strippedGot, 8, 8)

	if got := readRawMedia(t, db, "oversized").Thumb; got != "" {
		t.Errorf("oversized thumbnail = %q, want cleared", got)
	}
	if got := readRawMedia(t, db, "invalid").Thumb; got != "" {
		t.Errorf("invalid-base64 thumbnail = %q, want cleared", got)
	}
	if got := readRawMedia(t, db, "none").Thumb; got != "" {
		t.Errorf("media with no thumbnail = %q, want left alone", got)
	}

	// Idempotent: re-running the repair directly, bypassing migrate's
	// own version gate the same way the GIF, audio and pin-order tests
	// above do, must not touch an already-repaired row again.
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := sanitizeLegacyThumbnails(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if got := readRawMedia(t, db, "stripped").Thumb; got != strippedGot {
		t.Errorf("re-running the repair changed an already-repaired thumbnail: %q, want %q", got, strippedGot)
	}
}

// TestMigrate_BackfillsReplySenderID confirms the migration that fills
// in a reply quote's own stored sender id, added so RefreshSenderName
// can scope a name correction to the sender who actually sent the
// quoted message instead of trusting a message's remote id, which is
// only unique within one conversation (see sendernames.go and
// docs/decisions.md). A quote resolves against the quoted message
// stored in its own conversation; one whose remote id only exists in
// a different conversation is left with no sender id at all, exactly
// as an unresolved reply already is, rather than guessed from the
// wrong conversation's message.
func TestMigrate_BackfillsReplySenderID(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "legacy-replies.db"))
	// The reply sender id backfill is fifth-from-last, appended after
	// the thumbnail repair above and before the sender/title verified
	// columns, the conversation phone column, the verified-by-default
	// reset and the contact verified and phone columns.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-5]); err != nil {
		t.Fatal(err)
	}

	insertRawReplyMessage(t, db, rawReplyMessage{id: "quoted", conversationID: "c1", remoteID: "r1", senderID: "r-alex"})
	insertRawReplyMessage(t, db, rawReplyMessage{id: "resolvable", conversationID: "c1", remoteID: "r2", replyTo: `{"remoteId":"r1"}`})
	insertRawReplyMessage(t, db, rawReplyMessage{id: "already-set", conversationID: "c1", remoteID: "r3", replyTo: `{"remoteId":"r1","senderId":"r-someone-else"}`})
	// "other-chat" shares remote id "r1" with "quoted" above, but lives
	// in a different conversation, c2 - exactly the natural collision
	// (or a reused id) the fix must not cross.
	insertRawReplyMessage(t, db, rawReplyMessage{id: "other-chat", conversationID: "c2", remoteID: "r1", senderID: "r-other-chat-sender"})
	insertRawReplyMessage(t, db, rawReplyMessage{id: "cross-conversation-quote", conversationID: "c2", remoteID: "r4", replyTo: `{"remoteId":"r1"}`})
	insertRawReplyMessage(t, db, rawReplyMessage{id: "truly-unresolvable", conversationID: "c3", remoteID: "r5", replyTo: `{"remoteId":"no-such-message"}`})

	if err := migrate(t.Context(), db, migrations); err != nil {
		t.Fatal(err)
	}

	if got := readRawReplyTo(t, db, "resolvable"); got.SenderID != "r-alex" {
		t.Errorf("resolvable quote's SenderID = %q, want it resolved within its own conversation, r-alex", got.SenderID)
	}

	if got := readRawReplyTo(t, db, "already-set"); got.SenderID != "r-someone-else" {
		t.Errorf("already-set quote's SenderID = %q, want it left alone", got.SenderID)
	}

	// "cross-conversation-quote" is in c2, which has its own message
	// under remote id "r1" ("other-chat"): it must resolve to that
	// message's own sender, never to "quoted"'s sender in c1, even
	// though both conversations share the same remote id.
	if got := readRawReplyTo(t, db, "cross-conversation-quote"); got.SenderID != "r-other-chat-sender" {
		t.Errorf("cross-conversation-quote's SenderID = %q, want it resolved within its own conversation c2, r-other-chat-sender, never c1's r-alex", got.SenderID)
	}

	if got := readRawReplyTo(t, db, "truly-unresolvable"); got.SenderID != "" {
		t.Errorf("truly-unresolvable quote's SenderID = %q, want it left empty", got.SenderID)
	}

	// Idempotent: re-running the backfill directly, bypassing migrate's
	// own version gate, must not touch an already-backfilled row again.
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := backfillReplySenderIDs(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if got := readRawReplyTo(t, db, "resolvable"); got.SenderID != "r-alex" {
		t.Errorf("re-running the backfill changed resolvable's SenderID: %q, want r-alex", got.SenderID)
	}
}

// TestStaleQuotes_SkipsAnUnparsableReplyToJSON confirms
// RefreshSenderName's own staleQuotes (sendernames.go) skips a stored
// reply_to that does not even decode as JSON, rather than failing the
// whole rename: a row this malformed can never come from the store's
// own encodeReply, so this is tested directly against a raw row, the
// only way to produce one.
func TestStaleQuotes_SkipsAnUnparsableReplyToJSON(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "unparsable-reply.db"))
	if err := migrate(t.Context(), db, migrations); err != nil {
		t.Fatal(err)
	}

	insertRawReplyMessage(t, db, rawReplyMessage{id: "broken", conversationID: "c1", replyTo: "not valid json at all"})
	insertRawReplyMessage(t, db, rawReplyMessage{id: "fine", conversationID: "c1", replyTo: `{"remoteId":"1","senderId":"r-alex","senderName":"WhatsApp user"}`})

	rows, err := db.QueryContext(t.Context(), `SELECT id, reply_to FROM messages ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}

	fixes, err := staleQuotes(rows, "r-alex", "Alex Chen", true)
	if err != nil {
		t.Fatalf("staleQuotes: %v", err)
	}
	if len(fixes) != 1 || fixes[0].id != "fine" {
		t.Errorf("staleQuotes fixes = %+v, want only the parsable row fixed, the broken one skipped", fixes)
	}
}

// rawReplyMessage is one row insertRawReplyMessage writes directly,
// bypassing the store's own encodeReply: the exact pre-fix shape a
// reply was stored under before domain.Reply.SenderID existed.
type rawReplyMessage struct {
	id, conversationID, remoteID, senderID, replyTo string
}

// insertRawReplyMessage inserts a minimal message row from m.
func insertRawReplyMessage(t *testing.T, db *sql.DB, m rawReplyMessage) {
	t.Helper()

	_, err := db.ExecContext(t.Context(), `INSERT INTO messages(id,conversation_id,remote_id,sender_id,text,outgoing,status,created,reply_to)
		VALUES(?,?,?,?,'reply',0,'received',1,?)`, m.id, m.conversationID, m.remoteID, m.senderID, m.replyTo)
	if err != nil {
		t.Fatal(err)
	}
}

// readRawReplyTo reads and decodes id's stored reply_to column
// directly, bypassing the store's own public API, which this
// package's own migration tests cannot otherwise reach a pre-migration
// row through.
func readRawReplyTo(t *testing.T, db *sql.DB, id string) domain.Reply {
	t.Helper()

	var encoded string
	if err := db.QueryRowContext(t.Context(), `SELECT reply_to FROM messages WHERE id=?`, id).Scan(&encoded); err != nil {
		t.Fatal(err)
	}

	var reply domain.Reply
	if err := json.Unmarshal([]byte(encoded), &reply); err != nil {
		t.Fatalf("reply_to for %s does not decode: %v", id, err)
	}

	return reply
}

// fakePNGThumb returns a plain, solid-colour PNG of the given size,
// base64-encoded exactly as domain.Media.Thumb stores one.
func fakePNGThumb(width, height int) string {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)))

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// fakePNGThumbWithChunk returns fakePNGThumb's image with one extra
// chunk spliced in right after IHDR - exactly where a sender-supplied
// zTXt or iCCP chunk sits in a real file - so sanitizing it actually
// strips something rather than leaving it byte-for-byte unchanged.
func fakePNGThumbWithChunk(width, height int, kind string, payload []byte) string {
	data, err := base64.StdEncoding.DecodeString(fakePNGThumb(width, height))
	if err != nil {
		panic(err) // unreachable: fakePNGThumb's own output is always valid base64
	}

	const ihdrEnd = 8 + 8 + 13 + 4 // signature + length/type + IHDR body + CRC
	var out bytes.Buffer
	out.Write(data[:ihdrEnd])
	writeTestPNGChunk(&out, kind, payload)
	out.Write(data[ihdrEnd:])

	return base64.StdEncoding.EncodeToString(out.Bytes())
}

// writeTestPNGChunk appends one well-formed PNG chunk - length, type,
// payload and a correct CRC - to out.
func writeTestPNGChunk(out *bytes.Buffer, kind string, payload []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	out.Write(length[:])
	out.WriteString(kind)
	out.Write(payload)

	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(append([]byte(kind), payload...)))
	out.Write(crc[:])
}

// requireDecodesTo fails the test unless thumb, base64-encoded, decodes
// to an image of exactly width by height: sanitizing a thumbnail must
// never change what it depicts, only strip what rendering never reads.
func requireDecodesTo(t *testing.T, thumb string, width, height int) {
	t.Helper()

	data, err := base64.StdEncoding.DecodeString(thumb)
	if err != nil {
		t.Fatalf("thumbnail is not valid base64: %v", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != width || cfg.Height != height {
		t.Fatalf("decoded = %+v, %v; want %dx%d", cfg, err, width, height)
	}
}

// insertRawAccount inserts a minimal account row, bypassing the store's
// own API, which this package's migration tests run before a full
// migration they are testing has even applied.
func insertRawAccount(t *testing.T, db *sql.DB, id string) {
	t.Helper()

	if _, err := db.ExecContext(t.Context(), `INSERT INTO accounts(id,service,name) VALUES(?,'whatsapp',?)`, id, id); err != nil {
		t.Fatal(err)
	}
}

// rawConversation is one row insertRawConversation writes directly,
// bypassing the store's own API: the exact shape an older build left
// behind, every already-pinned conversation sharing pin_order's bare
// column default, so there is no field for it here at all.
type rawConversation struct {
	id, accountID string
	pinned        bool
	lastActivity  int
}

// insertRawConversation inserts a minimal conversation row from c.
func insertRawConversation(t *testing.T, db *sql.DB, c rawConversation) {
	t.Helper()

	_, err := db.ExecContext(t.Context(),
		`INSERT INTO conversations(id,account_id,remote_id,kind,title,pinned,pin_order,last_activity)
			VALUES(?,?,?,'direct',?,?,0,?)`,
		c.id, c.accountID, c.id, c.id, c.pinned, c.lastActivity)
	if err != nil {
		t.Fatal(err)
	}
}

// readRawPinOrder reads one conversation's stored pin_order directly.
func readRawPinOrder(t *testing.T, db *sql.DB, id string) int {
	t.Helper()

	var order int
	if err := db.QueryRowContext(t.Context(), `SELECT pin_order FROM conversations WHERE id=?`, id).Scan(&order); err != nil {
		t.Fatal(err)
	}

	return order
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
