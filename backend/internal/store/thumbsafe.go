package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// sanitizedThumb runs thumb - a message's preview image exactly as
// domain.Media.Thumb stores and the protocol sends it, base64 - back
// through imagesafe's own bound for one (see imagesafe.Thumbnail), the
// same check a connector already gives a thumbnail the moment it
// receives it. scanMessage calls this on every row it reads, so a
// thumbnail written before this check existed, or by any future write
// path that forgets to run it, is still caught here before it ever
// reaches the UI (see docs/decisions.md). It returns thumb unchanged
// when sanitizing it changes nothing, a re-encoded, stripped version
// when it does, or "" when thumb fails outright: not valid base64, not
// a recognized image format, or over the bound - never the original
// bytes in that case.
func sanitizedThumb(thumb string) string {
	if thumb == "" {
		return ""
	}

	data, err := base64.StdEncoding.DecodeString(thumb)
	if err != nil {
		return ""
	}

	safe, ok := imagesafe.Sanitize(data, imagesafe.Thumbnail)
	if !ok {
		return ""
	}

	return base64.StdEncoding.EncodeToString(safe)
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

	safe := sanitizedThumb(m.Thumb)
	if safe == m.Thumb {
		return media, false
	}

	m.Thumb = safe
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
