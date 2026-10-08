package whatsapp

// chatalias_test.go checks the chat_aliases table directly: chatID's
// own tests in normalize_test.go cover the behaviour it builds on top
// of this, but not every branch of resolveChatAlias and putChatAlias
// themselves (a database closed too early, a lookup that only finds a
// match on its second alias).

import "testing"

func TestResolveChatAlias_ReturnsEmptyWhenNothingIsSavedYet(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)

	got, err := media.resolveChatAlias(t.Context(), []string{"987654@lid"})
	if err != nil || got != "" {
		t.Errorf("resolveChatAlias = %q, err=%v; want \"\", nil for an alias never saved", got, err)
	}
}

func TestResolveChatAlias_FindsAMatchOnASecondAliasWhenTheFirstMisses(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putChatAlias(t.Context(), "15551234567@s.whatsapp.net", "15551234567@s.whatsapp.net"); err != nil {
		t.Fatal(err)
	}

	got, err := media.resolveChatAlias(t.Context(), []string{"987654@lid", "15551234567@s.whatsapp.net"})
	if err != nil || got != "15551234567@s.whatsapp.net" {
		t.Errorf("resolveChatAlias = %q, err=%v; want the canonical id found under the second alias", got, err)
	}
}

func TestPutChatAlias_ReplacesWhateverWasSavedForItBefore(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putChatAlias(t.Context(), "987654@lid", "wrong@s.whatsapp.net"); err != nil {
		t.Fatal(err)
	}
	if err := media.putChatAlias(t.Context(), "987654@lid", "15551234567@s.whatsapp.net"); err != nil {
		t.Fatal(err)
	}

	got, err := media.resolveChatAlias(t.Context(), []string{"987654@lid"})
	if err != nil || got != "15551234567@s.whatsapp.net" {
		t.Errorf("resolveChatAlias = %q, err=%v; want the later save to win", got, err)
	}
}

func TestResolveChatAlias_ErrorsOnAClosedDatabase(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.close(); err != nil {
		t.Fatal(err)
	}

	if _, err := media.resolveChatAlias(t.Context(), []string{"987654@lid"}); err == nil {
		t.Error("resolveChatAlias on a closed database succeeded, want an error")
	}
}

func TestPutChatAlias_ErrorsOnAClosedDatabase(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.close(); err != nil {
		t.Fatal(err)
	}

	if err := media.putChatAlias(t.Context(), "987654@lid", "15551234567@s.whatsapp.net"); err == nil {
		t.Error("putChatAlias on a closed database succeeded, want an error")
	}
}

// TestEnsureChatAliasTable_ErrorsWhenTheStatementFails confirms the
// wrapping error path: a database that cannot execute at all (closed
// before this call) reports an error rather than succeeding silently.
func TestEnsureChatAliasTable_ErrorsWhenTheStatementFails(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.close(); err != nil {
		t.Fatal(err)
	}

	if err := ensureChatAliasTable(t.Context(), media.db); err == nil {
		t.Error("ensureChatAliasTable on a closed database succeeded, want an error")
	}
}
