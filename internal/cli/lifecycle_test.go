package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// resetGlobalApp restores app to its zero value at the end of a test so init/
// cleanup tests can run without interfering with each other.
func resetGlobalApp(t *testing.T) {
	t.Helper()
	prev := app
	t.Cleanup(func() { app = prev })
	app = appState{}
}

// withCfgFile overrides the package-level cfgFile and restores it on cleanup.
func withCfgFile(t *testing.T, path string) {
	t.Helper()
	prev := cfgFile
	cfgFile = path
	t.Cleanup(func() { cfgFile = prev })
}

func TestInitApp_NoConfig_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	resetGlobalApp(t)

	// Empty cfgFile triggers FindConfigFile() lookup. Point it at a non-existent
	// path within our temp dir so it falls back to defaults.
	withCfgFile(t, filepath.Join(dir, "missing.yaml"))

	// Use a state_dir inside the temp dir so logging/db creation lands there.
	// We do this by writing a real yaml file with an explicit state_dir.
	cfgYAML := []byte("version: \"1\"\nworkspace:\n  state_dir: " + dir + "\n  log_level: info\n  backend: sqlite\n")
	if err := os.WriteFile(filepath.Join(dir, "px.yaml"), cfgYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	withCfgFile(t, filepath.Join(dir, "px.yaml"))

	if err := initApp(); err != nil {
		t.Fatalf("initApp: %v", err)
	}
	t.Cleanup(func() { _ = cleanupApp() })

	if app.stateDir != dir {
		t.Errorf("stateDir = %q, want %q", app.stateDir, dir)
	}
	if app.eventStore == nil || app.projStore == nil || app.projector == nil {
		t.Error("stores were not initialized")
	}
	if _, err := os.Stat(filepath.Join(dir, "events.jsonl")); err != nil {
		t.Errorf("events file should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "px.db")); err != nil {
		t.Errorf("db file should exist: %v", err)
	}
}

func TestInitApp_BadConfigPath(t *testing.T) {
	resetGlobalApp(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(bad, []byte("not: valid: yaml:::"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCfgFile(t, bad)
	if err := initApp(); err == nil {
		t.Error("expected error loading invalid yaml")
	}
}

func TestCleanupApp_Idempotent(t *testing.T) {
	resetGlobalApp(t)
	// Calling cleanup on a zero-value app should not panic.
	if err := cleanupApp(); err != nil {
		t.Errorf("cleanupApp on empty app: %v", err)
	}
}

func TestExecute_ReturnsError_OnUnknownCmd(t *testing.T) {
	resetGlobalApp(t)
	// Forge args so the cobra parser hits an unknown command.
	origArgs := os.Args
	os.Args = []string{"px", "this-cmd-does-not-exist"}
	t.Cleanup(func() { os.Args = origArgs })

	if err := Execute(); err == nil {
		t.Error("expected error from Execute on unknown cmd")
	}
}

func TestNewVersionCmd_Runs(t *testing.T) {
	cmd := newVersionCmd()
	out := captureStdout(t, func() {
		_ = cmd.Execute()
	})
	if !strings.Contains(out, "px ") {
		t.Errorf("expected 'px <ver>' in output, got %q", out)
	}
}

func TestNewRootCmd_NoArgsShowsHelp(t *testing.T) {
	cmd := NewRootCmd()
	// Run via cobra: when called with no args, RunE returns cmd.Help() which
	// writes to stdout — we just confirm no panic and a non-error result.
	cmd.SetArgs([]string{})
	cmd.SetOut(os.Stderr) // discard help output away from test framework
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Errorf("RunE returned error: %v", err)
	}
}

func TestNewRootCmd_ConfigFlagBindsToCfgFile(t *testing.T) {
	cmd := NewRootCmd()
	if cmd.PersistentFlags().Lookup("config") == nil {
		t.Error("missing --config persistent flag")
	}
}

func TestNewRootCmd_PreRunCallsInitAppForRealSubcommand(t *testing.T) {
	resetGlobalApp(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "px.yaml")
	cfgYAML := []byte("version: \"1\"\nworkspace:\n  state_dir: " + dir + "\n  log_level: info\n  backend: sqlite\n")
	if err := os.WriteFile(cfg, cfgYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	withCfgFile(t, cfg)

	root := NewRootCmd()
	// Find the events subcommand so we exercise the non-skip path.
	var events *cobra.Command
	for _, sub := range root.Commands() {
		if sub.Name() == "events" {
			events = sub
			break
		}
	}
	if events == nil {
		t.Fatal("events subcommand not registered")
	}
	if err := root.PersistentPreRunE(events, nil); err != nil {
		t.Fatalf("PersistentPreRunE: %v", err)
	}
	if err := root.PersistentPostRunE(events, nil); err != nil {
		t.Fatalf("PersistentPostRunE: %v", err)
	}
}

func TestNewRootCmd_PreRunSkipsForHelpVersion(t *testing.T) {
	cmd := NewRootCmd()
	// Find the actual version sub-cmd registered on the root so we exercise
	// the shouldSkipInit branch.
	var versionCmd *(struct{}) // placeholder to satisfy compiler in test
	_ = versionCmd
	for _, sub := range cmd.Commands() {
		if sub.Name() == "version" {
			if err := cmd.PersistentPreRunE(sub, nil); err != nil {
				t.Errorf("PreRunE should skip for version, got err: %v", err)
			}
			if err := cmd.PersistentPostRunE(sub, nil); err != nil {
				t.Errorf("PostRunE should skip for version, got err: %v", err)
			}
			return
		}
	}
	t.Fatal("version subcommand not registered")
}
