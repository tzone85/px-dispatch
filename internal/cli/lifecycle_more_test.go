package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitApp_CreateStateDirError(t *testing.T) {
	resetGlobalApp(t)

	// Point state_dir at a path under a read-only parent so MkdirAll fails.
	parentDir := t.TempDir()
	if err := os.Chmod(parentDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parentDir, 0o755) })

	stateDir := filepath.Join(parentDir, "cannot-create", "deep", "path")
	cfgYAML := []byte("version: \"1\"\nworkspace:\n  state_dir: " + stateDir + "\n  log_level: info\n  backend: sqlite\n")

	cfg := filepath.Join(t.TempDir(), "px.yaml")
	if err := os.WriteFile(cfg, cfgYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	withCfgFile(t, cfg)

	if err := initApp(); err == nil {
		t.Skip("MkdirAll succeeded — environment ignores permissions (e.g. root user); skipping")
	}
}

func TestCleanupApp_PartiallyInitialized(t *testing.T) {
	resetGlobalApp(t)

	dir := t.TempDir()
	cfg := filepath.Join(dir, "px.yaml")
	cfgYAML := []byte("version: \"1\"\nworkspace:\n  state_dir: " + dir + "\n  log_level: info\n  backend: sqlite\n")
	if err := os.WriteFile(cfg, cfgYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	withCfgFile(t, cfg)

	if err := initApp(); err != nil {
		t.Fatalf("initApp: %v", err)
	}

	// Close both stores ahead of cleanupApp so it hits both error-logging
	// branches.
	if err := app.projStore.Close(); err != nil {
		t.Fatalf("manual close proj: %v", err)
	}
	if err := app.eventStore.Close(); err != nil {
		t.Fatalf("manual close event: %v", err)
	}
	if err := cleanupApp(); err != nil {
		t.Errorf("cleanupApp swallowed normal errors but returned: %v", err)
	}
}

func TestExecute_DisplaysHelp(t *testing.T) {
	resetGlobalApp(t)
	origArgs := os.Args
	os.Args = []string{"px", "--help"}
	t.Cleanup(func() { os.Args = origArgs })

	// Help returns nil error.
	_ = Execute()
}
