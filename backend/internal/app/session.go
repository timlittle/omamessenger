package app

import (
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/app/policy"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

type settings struct {
	notifications       bool
	notificationPreview bool
	demoChatter         bool
}

// session is the UI state the application reacts to: the user's settings and
// which conversation, if any, they are looking at.
type session struct {
	mu           sync.RWMutex
	settings     settings
	focused      string
	windowActive bool
}

func newSession() *session {
	return &session{settings: settings{notifications: true, notificationPreview: true, demoChatter: true}}
}

func (s *session) setFocus(conversationID string, windowActive bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.focused, s.windowActive = conversationID, windowActive
}

func (s *session) apply(next settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = next
}

// policyInput describes message m arriving in conv under the current state.
func (s *session) policyInput(conv domain.Conversation, m domain.Message) policy.Input {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return policy.Input{
		Notifications: s.settings.notifications,
		Preview:       s.settings.notificationPreview,
		Muted:         conv.Muted,
		Focused:       s.focused == conv.ID,
		WindowActive:  s.windowActive,
		Kind:          conv.Kind,
		Sender:        m.SenderName,
		Title:         conv.Title,
		Text:          m.Text,
	}
}
