package app

import (
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// publisher sends C3 events to the UI, one at a time, and owns the rule that
// unread.changed is emitted only when the total actually changes.
type publisher struct {
	repo Repository
	emit Emit
	mu   sync.Mutex
}

type unreadEvent struct {
	Total int `json:"total"`
}

type typingEvent struct {
	ConversationID string `json:"conversationId"`
	Name           string `json:"name"`
	Active         bool   `json:"active"`
}

func (p *publisher) send(name string, data any) {
	if p.emit == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.emit(name, data)
}

// unreadTotal is the current total, or 0 when it cannot be read.
func (p *publisher) unreadTotal() int {
	total, err := p.repo.UnreadTotal()
	if err != nil {
		return 0
	}
	return total
}

// unreadChanged emits unread.changed if the total differs from before.
func (p *publisher) unreadChanged(before int) {
	if after := p.unreadTotal(); after != before {
		p.send("unread.changed", unreadEvent{Total: after})
	}
}

// conversation reloads a conversation and emits conversation.updated.
func (p *publisher) conversation(id string) (domain.Conversation, error) {
	c, err := p.repo.Conversation(id)
	if err != nil {
		return c, err
	}
	p.send("conversation.updated", c)
	return c, nil
}
