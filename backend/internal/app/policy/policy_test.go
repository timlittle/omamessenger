package policy_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app/policy"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// everyInput returns an arrival for every combination of the five flags,
// in both a direct chat and a group.
func everyInput() []policy.Input {
	var inputs []policy.Input
	for bits := range 32 {
		for _, kind := range []string{domain.KindDirect, domain.KindGroup} {
			inputs = append(inputs, policy.Input{
				Notifications: bits&1 != 0,
				Preview:       bits&2 != 0,
				Muted:         bits&4 != 0,
				Focused:       bits&8 != 0,
				WindowActive:  bits&16 != 0,
				Kind:          kind,
				Sender:        "Priya",
				Title:         "Climbing Crew",
				Text:          "see you at 7",
			})
		}
	}

	return inputs
}

func TestMarkReadOnArrival_OnlyWhenLookingAtIt(t *testing.T) {
	t.Parallel()

	for _, in := range everyInput() {
		want := in.Focused && in.WindowActive
		if got := policy.MarkReadOnArrival(in); got != want {
			t.Errorf("%+v: MarkReadOnArrival = %t, want %t", in, got, want)
		}
	}
}

func TestShouldNotify_UnlessOffMutedOrLookingAtIt(t *testing.T) {
	t.Parallel()

	for _, in := range everyInput() {
		lookingAtIt := in.Focused && in.WindowActive
		want := in.Notifications && !in.Muted && !lookingAtIt
		if got := policy.ShouldNotify(in); got != want {
			t.Errorf("%+v: ShouldNotify = %t, want %t", in, got, want)
		}
	}
}

func TestNotification_NamesGroupsAndHidesTextWithoutPreview(t *testing.T) {
	t.Parallel()

	for _, in := range everyInput() {
		wantTitle, wantBody := "Priya", "New message"
		if in.Kind == domain.KindGroup {
			wantTitle = "Priya · Climbing Crew"
		}

		if in.Preview {
			wantBody = "see you at 7"
		}

		if title, body := policy.Notification(in); title != wantTitle || body != wantBody {
			t.Errorf("%+v: Notification = %q, %q; want %q, %q", in, title, body, wantTitle, wantBody)
		}
	}
}
