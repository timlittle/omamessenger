package fake

import (
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// demoScripts is a small, curated set of fake conversations for the
// README demo recording: a handful of neutral, plausible names, not the
// fuller fixture scripts uses for everything else, so the list looks
// clean in a short clip and connecting is fast enough for a short
// recording. One conversation carries both a loaded photo and a link
// preview, so a single open shows both without scrolling.
var demoScripts = []accountScript{
	{
		account:      domain.Account{ID: "wa-demo", Service: domain.ServiceWhatsApp, Name: "Personal"},
		connectDelay: 150 * time.Millisecond,
		conversations: []conversationScript{
			{
				remoteID: "wa:priya-patel", title: "Priya Patel", kind: domain.KindDirect, count: 3, unread: 1,
				lastPhoto: true,
				texts: []string{
					"Hey, are you free Thursday?",
					"Here's the venue: https://example.com/venue",
					"Can't wait!",
				},
				linkPreviewText: "Here's the venue: https://example.com/venue",
			},
			{
				remoteID: "wa:design-team", title: "Design Team", kind: domain.KindGroup, members: 6, count: 4, unread: 2,
				groupSenders: []string{"Noah", "Mia"},
				texts:        []string{"Can we push the review to Friday?", "I've updated the mockups", "Sounds good to me"},
			},
			{
				remoteID: "wa:jordan-lee", title: "Jordan Lee", kind: domain.KindDirect, count: 3,
				texts: []string{"Thanks for the update", "Let me know if anything changes", "Will do"},
			},
			{
				remoteID: "wa:weekend-hike", title: "Weekend Hike", kind: domain.KindGroup, members: 5, count: 5, unread: 3, muted: true,
				groupSenders: []string{"Sam", "Lee"},
				texts:        []string{"Meet at the trailhead at 8", "I'll bring the first aid kit", "Weather looks good"},
			},
		},
	},
}
