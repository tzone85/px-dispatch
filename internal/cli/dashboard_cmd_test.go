package cli

import (
	"context"
	"testing"
	"time"
)

func TestOpenBrowser_DoesNotPanic(t *testing.T) {
	// Best-effort fire-and-forget: must not panic on any platform.
	openBrowser("http://localhost:0")
}

func TestOpenBrowser_AllPlatforms(t *testing.T) {
	prev := browserGOOS
	t.Cleanup(func() { browserGOOS = prev })

	for _, goos := range []string{"darwin", "linux", "windows", "plan9"} {
		t.Run(goos, func(t *testing.T) {
			browserGOOS = func() string { return goos }
			// No panic on any branch — process spawn may fail silently.
			openBrowser("http://localhost:0")
		})
	}
}

func TestRunWebDashboard_CancelledContext(t *testing.T) {
	setupTestApp(t)

	// Pick an ephemeral port by binding 0. The server will start then exit
	// when the context is cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so the server returns quickly

	done := make(chan error, 1)
	go func() {
		done <- runWebDashboard(ctx, 0, "127.0.0.1")
	}()
	select {
	case <-done:
		// Expected: server returns once ctx is cancelled.
	case <-time.After(3 * time.Second):
		t.Fatal("runWebDashboard did not return after context cancel")
	}
}
