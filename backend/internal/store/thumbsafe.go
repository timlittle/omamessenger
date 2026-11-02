package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// verifyThumb returns media with its own Thumb passed through
// sanitizedThumb and ThumbWidth/ThumbHeight filled from the real,
// decoded result, so a message a write path returns - the one
// app.Ingest publishes straight to the UI as message.added or
// message.updated, with no read of the row back out of the database
// in between - already carries the same verified size a later read
// through scanMessage would give it. Without this, a message
// delivered live, backfilled from history or edited reached the UI
// with Thumb's own box alone, because sanitizedThumb only ever ran
// when a row was read back, never on the write that first produced
// the value a connector's own report actually carries (see
// docs/decisions.md). media is returned unchanged when it is nil or
// carries no thumbnail at all.
func verifyThumb(media *domain.Media) *domain.Media {
	if media == nil || media.Thumb == "" {
		return media
	}

	verified := *media
	verified.Thumb, verified.ThumbWidth, verified.ThumbHeight = sanitizedThumb(media.Thumb)

	return &verified
}

// sanitizedThumb runs thumb - a message's preview image exactly as
// domain.Media.Thumb stores and the protocol sends it, base64 - back
// through imagesafe's own bound for one (see imagesafe.Thumbnail), the
// same check a connector already gives a thumbnail the moment it
// receives it. scanMessage calls this on every row it reads, and
// verifyThumb calls it on every write, so a thumbnail written before
// this check existed, or by any future write path that forgets to run
// it, is still caught here before it ever reaches the UI (see
// docs/decisions.md). It returns thumb unchanged when sanitizing it
// changes nothing, a re-encoded, stripped version when it does, or ""
// when thumb fails outright: not valid base64, not a recognized image
// format, or over the bound - never the original bytes in that case.
// width and height are the thumbnail's own real dimensions, as
// imagesafe.Sanitize's own parse of it just confirmed them, for
// domain.Media.ThumbWidth/ThumbHeight: the UI must size its own decode
// of this same thumb from these, never from a message's claimed photo
// or sticker size, which need not match what the thumbnail bytes
// actually decode to (see docs/decisions.md). Both are zero whenever
// thumb is "".
func sanitizedThumb(thumb string) (safe string, width, height int) {
	if thumb == "" {
		return "", 0, 0
	}

	data, err := base64.StdEncoding.DecodeString(thumb)
	if err != nil {
		return "", 0, 0
	}

	out, w, h, ok := imagesafe.Sanitize(data, imagesafe.Thumbnail)
	if !ok {
		return "", 0, 0
	}

	return base64.StdEncoding.EncodeToString(out), w, h
}

// thumbRow is one candidate row sanitizeLegacyThumbnails considers: its
// rowid, for the UPDATE that follows, and its media column exactly as
// stored.
type thumbRow struct {
	rowid int64
	media string
}

// scanThumbRow reads one row selected by sanitizeLegacyThumbnails's own
// query.
func scanThumbRow(row scanner) (thumbRow, error) {
	var r thumbRow
	err := row.Scan(&r.rowid, &r.media)

	return r, err
}

// repairMediaThumb re-sanitizes the thumbnail inside media, a message's
// stored media JSON, through sanitizedThumb. changed is false, and
// media is returned unchanged, when the JSON cannot be parsed or carries
// no thumbnail at all - this never touches anything but that one field -
// or when sanitizing its thumbnail changes nothing.
func repairMediaThumb(media string) (repaired string, changed bool) {
	var m domain.Media
	if err := json.Unmarshal([]byte(media), &m); err != nil || m.Thumb == "" {
		return media, false
	}

	safe, width, height := sanitizedThumb(m.Thumb)
	if safe == m.Thumb && width == m.ThumbWidth && height == m.ThumbHeight {
		return media, false
	}

	m.Thumb, m.ThumbWidth, m.ThumbHeight = safe, width, height
	out, err := json.Marshal(&m)
	if err != nil {
		return media, false
	}

	return string(out), true
}

// sanitizeLegacyThumbnails repairs every message thumbnail stored
// before imagesafe.Sanitize existed, the one-time migration step that
// fixes data already on disk; scanMessage's own call to sanitizedThumb
// is what keeps guaranteeing this for every row from now on, including
// any a future bug lets through unsanitized (see docs/decisions.md). It
// is bounded by imagesafe.Thumbnail's own per-image decode limits and
// idempotent: a thumbnail sanitizedThumb already accepted sanitizes to
// the same bytes a second time, so a repeated run changes nothing.
func sanitizeLegacyThumbnails(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT rowid, media FROM messages WHERE media LIKE '%"thumb":%'`)
	if err != nil {
		return err
	}

	candidates, err := scanAll(rows, scanThumbRow)
	if err != nil {
		return err
	}

	for _, c := range candidates {
		repaired, changed := repairMediaThumb(c.media)
		if !changed {
			continue
		}

		if _, err := tx.ExecContext(ctx, `UPDATE messages SET media=? WHERE rowid=?`, repaired, c.rowid); err != nil {
			return err
		}
	}

	return nil
}
