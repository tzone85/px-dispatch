package dashboard

import (
	"testing"
	"time"
)

// timeoutChan returns a channel that fires after a long-enough deadline for
// dashboard tests that involve tea.Tick (which itself uses 2s).
func timeoutChan(t *testing.T) <-chan time.Time {
	t.Helper()
	return time.After(5 * time.Second)
}
