package sandbox

// limits_test.go pins maxJobBytes directly: it is a derived constant,
// not an independent fact of its own, so this exists to make a future
// change to any one of its four inputs show up here too, rather than
// silently drifting from the single number docs/decisions.md states.

import "testing"

// TestMaxJobBytes_MatchesTheDocumentedTotal checks maxJobBytes against
// the literal byte count docs/decisions.md states as the total memory
// and disk one job can reach: 4 GiB (maxAddressSpaceBytes) + 16 MiB
// (maxTmpBytes) + 16 MiB (maxDevShmBytes) + 512 MiB
// (maxOutputFileBytes).
func TestMaxJobBytes_MatchesTheDocumentedTotal(t *testing.T) {
	t.Parallel()

	const want = 4_865_392_640

	if maxJobBytes != want {
		t.Errorf("maxJobBytes = %d, want %d", maxJobBytes, want)
	}
}
