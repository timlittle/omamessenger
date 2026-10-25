package connector_test

// attachment_limit_test.go checks that the Manager routes an
// attachment limit lookup to the conversation's own connector, when it
// reports any, and falls back to "no limit reported" for one that does
// not or for an account with no connector at all.

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// withLimit is a connector that reports an attachment limit for a
// kind, recording which kind it was asked about.
type withLimit struct {
	fakeConnector
	asked []string
	limit int64
}

// AttachmentLimit reports the limit configured on the fake, recording
// the kind it was asked about.
func (c *withLimit) AttachmentLimit(kind string) int64 {
	c.asked = append(c.asked, kind)

	return c.limit
}

func TestAttachmentLimit_AsksConnectorsThatReportOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withLimit{fakeConnector: fakeConnector{id: "tg"}, limit: 4 << 30}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		limit, ok := m.AttachmentLimit("tg", domain.MediaFile)
		if !ok || limit != 4<<30 {
			t.Errorf("AttachmentLimit(tg) = %d, %v, want %d, true", limit, ok, int64(4<<30))
		}
		if len(tg.asked) != 1 || tg.asked[0] != domain.MediaFile {
			t.Errorf("asked = %v, want [%s]", tg.asked, domain.MediaFile)
		}

		if limit, ok := m.AttachmentLimit("plain", domain.MediaFile); ok || limit != 0 {
			t.Errorf("AttachmentLimit(connector without the capability) = %d, %v, want 0, false", limit, ok)
		}

		if limit, ok := m.AttachmentLimit("nobody", domain.MediaFile); ok || limit != 0 {
			t.Errorf("AttachmentLimit(unknown account) = %d, %v, want 0, false", limit, ok)
		}

		cancel()
		m.Wait()
	})
}
