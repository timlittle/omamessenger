package policy

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestEveryCombination checks all 2^5 boolean states for both kinds against
// the C3 rules written out independently.
func TestEveryCombination(t *testing.T) {
	for bits := 0; bits < 32; bits++ {
		for _, kind := range []string{domain.KindDirect, domain.KindGroup} {
			in := Input{
				Notifications: bits&1 != 0,
				Preview:       bits&2 != 0,
				Muted:         bits&4 != 0,
				Focused:       bits&8 != 0,
				WindowActive:  bits&16 != 0,
				Kind:          kind, Sender: "Priya", Title: "Climbing Crew", Text: "see you at 7",
			}
			lookingAtIt := in.Focused && in.WindowActive
			if got := MarkReadOnArrival(in); got != lookingAtIt {
				t.Errorf("%+v: MarkReadOnArrival = %t, want %t", in, got, lookingAtIt)
			}
			wantNotify := in.Notifications && !in.Muted && !lookingAtIt
			if got := ShouldNotify(in); got != wantNotify {
				t.Errorf("%+v: ShouldNotify = %t, want %t", in, got, wantNotify)
			}
			title, body := Notification(in)
			wantTitle, wantBody := "Priya", "New message"
			if kind == domain.KindGroup {
				wantTitle = "Priya · Climbing Crew"
			}
			if in.Preview {
				wantBody = "see you at 7"
			}
			if title != wantTitle || body != wantBody {
				t.Errorf("%+v: Notification = %q, %q; want %q, %q", in, title, body, wantTitle, wantBody)
			}
		}
	}
}
