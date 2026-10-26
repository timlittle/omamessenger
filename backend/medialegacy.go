package main

// medialegacy.go closes a gap a security review found in how the
// downloaded media cache and the outgoing attachment area treat a
// photo, sticker or GIF already on disk from an older run of this
// helper: cache.Cache.Fetch and cache.Outgoing's own files are served
// again on a cache hit with no further check, so a file written
// before imagesafe.Sanitize covered it (or before one of its own
// checks was tightened) would otherwise still reach a QML Image or
// AnimatedImage unsanitized after an upgrade - the exact class of bug
// already fixed once for stored thumbnails (see
// store.sanitizeLegacyThumbnails and docs/decisions.md). A video,
// voice note or file needs no equivalent sweep: each already gets its
// own kind-specific safety check on every single fetch, cache hit or
// not (see connector.SanitizeIfGIF, app.VideoPlayableForMedia,
// AudioPlayableForMedia and app.MediaRisk), so only an image's gap
// needed closing here.

import (
	"log"

	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// legacyMediaFormatVersion is bumped whenever imagesafe's own checks
// for an image already on disk change in a way a file written by an
// older version of this helper might not satisfy: revalidateLegacyMedia
// re-examines every image in a media area again the next time it
// finds an older or missing format marker there (see
// cache.Cache.FormatVersion and cache.Outgoing.FormatVersion).
const legacyMediaFormatVersion = 1

// legacyMediaArea names one media area's own format-marker and
// revalidation functions, so revalidateArea takes one argument
// instead of several separately named ones for each of the
// downloaded cache and the outgoing area.
type legacyMediaArea struct {
	name             string
	formatVersion    func() (int, error)
	setFormatVersion func(int) error
	revalidate       func(cache.RevalidateCheck) error
	check            cache.RevalidateCheck
}

// revalidateLegacyMedia re-checks every image already in caches'
// downloaded media cache and outgoing attachment area against
// imagesafe's current checks, once per area, the first time this
// helper starts with an older or missing format marker there (see
// cache.FormatVersion). serve calls this before srv.Start, so a cache
// hit for an image can never be served from a file this has not
// already checked: the one guarantee this whole sweep exists to make.
// A failure revalidating one area, or recording its marker, is
// logged and otherwise ignored, so a file this run could not check
// just gets checked again the next time the helper starts, rather
// than ever stopping the helper from starting at all.
func revalidateLegacyMedia(caches mediaCaches, logger *log.Logger) {
	revalidateArea(legacyMediaArea{
		name: "media", formatVersion: caches.downloaded.FormatVersion, setFormatVersion: caches.downloaded.SetFormatVersion,
		revalidate: caches.downloaded.Revalidate, check: connector.RevalidateDownloadedImage,
	}, logger)
	revalidateArea(legacyMediaArea{
		name: "outgoing media", formatVersion: caches.outgoing.FormatVersion, setFormatVersion: caches.outgoing.SetFormatVersion,
		revalidate: caches.outgoing.Revalidate, check: connector.RevalidateOutgoingImage,
	}, logger)
}

// revalidateArea runs area's own sweep once its own format marker is
// older than legacyMediaFormatVersion, or missing entirely, and
// records the current version once it has finished - even when the
// sweep itself reported a problem with one file, so a single bad file
// never leaves every other file in the area unchecked on every future
// start too.
func revalidateArea(area legacyMediaArea, logger *log.Logger) {
	version, err := area.formatVersion()
	if err != nil {
		logger.Printf("media: %s: could not read its format marker; revalidating anyway", area.name)
	} else if version >= legacyMediaFormatVersion {
		return
	}

	if err := area.revalidate(area.check); err != nil {
		logger.Printf("media: %s: revalidating legacy files failed", area.name)
	}

	if err := area.setFormatVersion(legacyMediaFormatVersion); err != nil {
		logger.Printf("media: %s: could not record its format marker", area.name)
	}
}
