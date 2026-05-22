package cli

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// withStubTeaRunner replaces the bubbletea program runner with one that
// returns the provided error without actually starting the terminal loop.
// This avoids the race conditions that arise when tea reads from os.Stdin
// while the test swaps it out.
func withStubTeaRunner(t *testing.T, err error) {
	t.Helper()
	prev := teaRunner
	teaRunner = func(_ *tea.Program) error { return err }
	t.Cleanup(func() { teaRunner = prev })
}

func TestRunTUIDashboard_Success(t *testing.T) {
	setupTestApp(t)
	withStubTeaRunner(t, nil)
	if err := runTUIDashboard(); err != nil {
		t.Errorf("runTUIDashboard: %v", err)
	}
}

func TestRunTUIDashboard_PropagatesError(t *testing.T) {
	setupTestApp(t)
	want := errors.New("tea failure")
	withStubTeaRunner(t, want)
	if err := runTUIDashboard(); !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
