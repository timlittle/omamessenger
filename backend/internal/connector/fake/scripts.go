package fake

import (
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// accountScript is one fake account and what it contains.
type accountScript struct {
	account       domain.Account
	connectDelay  time.Duration
	conversations []conversationScript
	contacts      []domain.Contact
}

// conversationScript describes a seeded conversation and how it behaves.
type conversationScript struct {
	remoteID string
	title    string
	kind     string
	members  int
	muted    bool

	// count messages are seeded, the last unread of them unread.
	count  int
	unread int

	// allOutgoing seeds only outgoing messages; lastOutgoing makes the
	// newest seeded message outgoing.
	allOutgoing  bool
	lastOutgoing bool

	// flaky fails the first send attempt of every message.
	flaky bool

	groupSenders []string
	texts        []string
	replies      []string
}

// scripts are the fake accounts, in the order the rail shows them.
var scripts = []accountScript{
	{
		account:      domain.Account{ID: "wa-personal", Service: domain.ServiceWhatsApp, Name: "Personal"},
		connectDelay: 600 * time.Millisecond,
		conversations: []conversationScript{
			{
				remoteID: "wa:mum", title: "Mum", kind: domain.KindDirect, count: 14,
				texts:   []string{"Call me when you're on your way.", "I made some soup for tomorrow", "Did you see the weather?"},
				replies: []string{"Just leaving now", "I will bring it over", "Thanks for letting me know"},
			},
			{
				remoteID: "wa:climbing-crew", title: "Climbing Crew", kind: domain.KindGroup, members: 5, count: 30, unread: 3,
				groupSenders: []string{"Priya", "Tom", "Jess", "Omar"},
				texts:        []string{"Saturday at the gym?", "I can get there around ten", "Anyone bringing the rope?"},
				replies:      []string{"Nice send!", "Route looks good", "See you at the wall"},
			},
			{
				remoteID: "wa:alex-chen", title: "Alex Chen", kind: domain.KindDirect, count: 6, unread: 1,
				texts:   []string{"Are we still on for lunch?", "I found a place near the station", "Here are the tickets: https://example.com/tickets"},
				replies: []string{"That works for me", "Just sent the details", "Thanks, see you there"},
			},
			{
				remoteID: "wa:sam-spotty", title: "Sam (spotty signal)", kind: domain.KindDirect, count: 4, flaky: true,
				texts:   []string{"Can you hear me now?", "Signal is dropping again", "I'll try from outside"},
				replies: []string{"It came through this time", "Sorry, lost connection", "Can you resend that?"},
			},
			{
				remoteID: "wa:flat-4b", title: "Flat 4B", kind: domain.KindGroup, members: 4, count: 12, unread: 5, muted: true,
				groupSenders: []string{"Rae", "Morgan", "Jamie"},
				texts:        []string{"Water bill is due Friday", "I can clean the kitchen", "Package arrived downstairs"},
				replies:      []string{"I'll take care of it", "Thanks for the heads up", "Whose parcel is it?"},
			},
			{
				remoteID: "wa:dentist", title: "Dr. Bartholomew Featherstonehaugh-Wainwright (Dentist)", kind: domain.KindDirect, count: 2,
				texts:   []string{"Your appointment is confirmed", "Please arrive ten minutes early"},
				replies: []string{"Thank you, see you then"},
			},
		},
		contacts: []domain.Contact{
			{RemoteID: "wa:contact-ben", Name: "Ben Okafor"},
			{RemoteID: "wa:contact-carla", Name: "Carla Ruiz"},
			{RemoteID: "wa:contact-dev", Name: "Dev Patel"},
		},
	},
	{
		account:      domain.Account{ID: "tg-personal", Service: domain.ServiceTelegram, Name: "Personal"},
		connectDelay: 600 * time.Millisecond,
		conversations: []conversationScript{
			{
				remoteID: "tg:nadia", title: "Nadia", kind: domain.KindDirect, count: 10, unread: 2,
				texts: []string{
					"How was the trip?", "The gallery was lovely",
					"The plan is set.\nMeet by the old gate.\nBring water.",
					"مرحبا، كيف حالك؟", "We should go again soon",
				},
				replies: []string{"I had a great time", "The new exhibit opens Friday", "أكيد، أراك قريباً"},
			},
			{
				remoteID: "tg:omarchy-users", title: "Omarchy Users", kind: domain.KindGroup, members: 2400, count: 150, unread: 12,
				groupSenders: []string{"Kai", "Mira", "Lee", "Nora", "Ash"},
				texts:        []string{"Anyone tried the new shell update?", "The plugin docs helped a lot", "I found a small theme issue"},
				replies:      []string{"I can reproduce that", "Thanks for sharing", "Fixed in the latest update"},
			},
			{
				remoteID: "tg:saved-messages", title: "Saved Messages", kind: domain.KindDirect, count: 5, allOutgoing: true,
				texts: []string{"Remember the book title", "groceries: coffee, oats, lemons", "idea: try the new trail"},
			},
		},
		contacts: []domain.Contact{
			{RemoteID: "tg:contact-elif", Name: "Elif Kaya"},
			{RemoteID: "tg:contact-femi", Name: "Femi Adeyemi"},
			{RemoteID: "tg:contact-greta", Name: "Greta Lind"},
		},
	},
	{
		account:      domain.Account{ID: "tg-work", Service: domain.ServiceTelegram, Name: "Work"},
		connectDelay: 2 * time.Second,
		conversations: []conversationScript{
			{
				remoteID: "tg:platform-team", title: "Platform Team", kind: domain.KindGroup, members: 9, count: 20, unread: 4,
				groupSenders: []string{"Jordan", "Hana", "Ivan", "Jo"},
				texts:        []string{"Deploy window is Thursday", "I'll update the runbook", "Can someone review the rollout?"},
				replies:      []string{"Looks good to me", "I can take that", "Let's sync after standup"},
			},
			{
				remoteID: "tg:jordan-manager", title: "Jordan (Manager)", kind: domain.KindDirect, count: 8, lastOutgoing: true,
				texts:   []string{"Let's talk about the project plan", "Could you send the estimate?", "Thanks, that answers my question"},
				replies: []string{"Will do", "I will send it this afternoon", "Sounds good"},
			},
		},
		contacts: []domain.Contact{
			{RemoteID: "tg:contact-hana", Name: "Hana Sato"},
			{RemoteID: "tg:contact-ivan", Name: "Ivan Petrov"},
			{RemoteID: "tg:contact-jo", Name: "Jo Park"},
		},
	},
}
