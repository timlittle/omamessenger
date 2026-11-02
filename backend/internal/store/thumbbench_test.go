package store_test

// thumbbench_test.go measures sanitizedThumb's own read-time cost
// once it does a full imagesafe.Reencode decode instead of a cheap
// structural check: the number docs/decisions.md records to justify
// whether a page of messages can still afford to re-sanitize every
// thumbnail on every read, or must move that cost to write time
// instead (see thumbsafe.go).

import (
	"fmt"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// BenchmarkMessages_PageOfThumbnails measures reading a page of
// messages back out, each carrying a thumbnail of the given size,
// through the store's public API: the same scanMessage path a
// conversation's own history scroll takes, so the number reported
// includes every read-time cost, not sanitizedThumb's alone.
func BenchmarkMessages_PageOfThumbnails(b *testing.B) {
	sizes := map[string][2]int{
		"typical stripped thumbnail (40x30)": {40, 30},
		"thumbnail cap (1024x1024)":          {1024, 1024},
	}

	for name, dims := range sizes {
		b.Run(name, func(b *testing.B) {
			s, conv := openChatStore(b)

			const pageSize = 50
			for i := 0; i < pageSize; i++ {
				m := domain.Message{
					ID: fmt.Sprintf("m%d", i), ConversationID: conv.ID, Text: "hi", Created: int64(i),
					Media: &domain.Media{Kind: domain.MediaPhoto, Thumb: fakeThumb(b, dims[0], dims[1])},
				}
				if _, _, err := s.AddMessage(b.Context(), m); err != nil {
					b.Fatal(err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := s.Messages(b.Context(), conv.ID, "", pageSize); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
