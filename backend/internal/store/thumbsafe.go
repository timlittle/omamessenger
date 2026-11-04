package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// verifyThumb runs media's Thumb through sanitizedThumb and fills
// ThumbWidth/ThumbHeight from the result, so a message a write path
// publishes straight to the UI (docs/decisions.md) carries the same
// verified thumbnail a later read would. media is returned unchanged
// when it is nil or has no thumbnail.
func verifyThumb(media *domain.Media) *domain.Media {
	if media == nil || media.Thumb == "" {
		return media
	}

	verified := *media
	verified.Thumb, verified.ThumbWidth, verified.ThumbHeight = sanitizedThumb(media.Thumb)

	return &verified
}

// sanitizedThumb re-runs thumb, a base64 preview image, through
// imagesafe.Sanitize, catching anything written before this check
// existed. It returns thumb unchanged, a re-encoded version, or "" (never
// the original bytes) when it is invalid, too large, or the re-encoded
// result exceeds domain.MaxInlineThumbLen. width and height are the
// decoded image's real dimensions, both zero when thumb is "".
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

	encoded := base64.StdEncoding.EncodeToString(out)
	if len(encoded) > domain.MaxInlineThumbLen {
		return "", 0, 0
	}

	return encoded, w, h
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
// stored media JSON. changed is false, and media is returned unchanged,
// when the JSON has no thumbnail or sanitizing it changes nothing.
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

// sanitizeLegacyThumbnails is a one-time migration that repairs every
// message thumbnail stored before imagesafe.Sanitize existed. It is
// idempotent: re-running it on an already-sanitized thumbnail changes
// nothing.
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
