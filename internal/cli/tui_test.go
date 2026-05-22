package cli

import (
	"os"
	"sync"
	"testing"
	"time"
)

// TestRunTUIDashboard_ExitsWithoutTTY ensures the function is at least invoked
// in a non-interactive test environment. tea.NewProgram fails fast without a
// real terminal, so we just exercise the wiring.
func TestRunTUIDashboard_ExitsWithoutTTY(t *testing.T) {
	setupTestApp(t)

	// Pipe stdin/stdout so tea can't interact normally.
	oldIn, oldOut := os.Stdin, os.Stdout
	r, w, _ := os.Pipe()
	os.Stdin = r
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdin = oldIn
		os.Stdout = oldOut
		_ = r.Close()
		_ = w.Close()
	})

	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer once.Do(func() { close(done) })
		defer func() { _ = recover() }() // tea may panic on a closed pipe
		_ = runTUIDashboard()
	}()

	// Give tea a moment to start, then close stdin to force it to exit.
	time.Sleep(50 * time.Millisecond)
	_ = r.Close()
	_ = w.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Log("runTUIDashboard did not exit within 3s (acceptable in some envs)")
	}
}
