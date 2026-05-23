package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/state"
)

// setupTestApp wires `app` to fresh in-tmpdir stores for the duration of a
// test. It restores the previous `app` and closes the stores on cleanup so
// tests cannot leak state to each other.
func setupTestApp(t *testing.T) string {
	t.Helper()

	prev := app
	t.Cleanup(func() { app = prev })

	dir := t.TempDir()

	es, err := state.NewFileStore(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatalf("file store: %v", err)
	}
	ps, err := state.NewSQLiteStore(filepath.Join(dir, "px.db"))
	if err != nil {
		es.Close()
		t.Fatalf("sqlite store: %v", err)
	}
	proj := state.NewProjector(ps, 16)
	proj.Start()

	t.Cleanup(func() {
		proj.Shutdown()
		ps.Close()
		es.Close()
	})

	app = appState{
		config:     config.Defaults(),
		eventStore: es,
		projStore:  ps,
		projector:  proj,
		stateDir:   dir,
	}
	return dir
}

// captureStdout redirects os.Stdout while fn runs and returns the captured
// output. Useful for asserting on commands that print directly via fmt.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	w.Close()
	<-done
	return buf.String()
}
