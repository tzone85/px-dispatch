package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/logging"
	"github.com/tzone85/px-dispatch/internal/state"
)

var (
	cfgFile string
	version = "dev"
)

// appState holds the initialized stores and config for subcommands.
type appState struct {
	config     config.Config
	eventStore *state.FileStore
	projStore  *state.SQLiteStore
	projector  *state.Projector
	logCleanup func()
	stateDir   string
}

var app appState

// skipInitCommands lists command names that should work without full state initialization.
var skipInitCommands = map[string]bool{
	"version": true,
	"help":    true,
}

// shouldSkipInit walks up the command tree to determine if initialization should be skipped.
func shouldSkipInit(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}
	if skipInitCommands[cmd.Name()] {
		return true
	}
	return false
}

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "px",
		Short: "px-dispatch — AI agent orchestration for the full SDLC",
		Long:  "Orchestrate autonomous AI agents from requirements to merged PRs.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if shouldSkipInit(cmd) {
				return nil
			}
			return initApp()
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if shouldSkipInit(cmd) {
				return nil
			}
			return cleanupApp()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./px.yaml)")
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newMigrateCmd())
	cmd.AddCommand(newEventsCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newCostCmd())
	cmd.AddCommand(newPlanCmd())
	cmd.AddCommand(newResumeCmd())
	cmd.AddCommand(newDashboardCmd())
	cmd.AddCommand(newAgentsCmd(), newConfigCmd(), newGCCmd(), newArchiveCmd())
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("px %s\n", version)
		},
	}
}

// initApp loads config, sets up logging, and opens all stores.
func initApp() error {
	// Resolve config file path.
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = config.FindConfigFile()
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	// Expand ~ in state directory so all downstream consumers get the real path.
	cfg.Workspace.StateDir = config.ExpandHome(cfg.Workspace.StateDir)
	app.config = cfg

	stateDir := cfg.Workspace.StateDir
	app.stateDir = stateDir

	// Set up logging.
	logDir := filepath.Join(stateDir, "logs")
	cleanup, err := logging.Setup(cfg.Workspace.LogLevel, logDir)
	if err != nil {
		return fmt.Errorf("setup logging: %w", err)
	}
	app.logCleanup = cleanup

	slog.Info("initializing px", "state_dir", stateDir, "log_level", cfg.Workspace.LogLevel)

	// Create state directory if needed.
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	// Open event store (append-only JSONL).
	eventsPath := filepath.Join(stateDir, "events.jsonl")
	eventStore, err := state.NewFileStore(eventsPath)
	if err != nil {
		return fmt.Errorf("open event store: %w", err)
	}
	app.eventStore = eventStore

	// Open projection store (SQLite with auto-migrations).
	dbPath := filepath.Join(stateDir, "px.db")
	projStore, err := state.NewSQLiteStore(dbPath)
	if err != nil {
		eventStore.Close()
		return fmt.Errorf("open projection store: %w", err)
	}
	app.projStore = projStore

	// Create and start the async projector.
	projector := state.NewProjector(projStore, 256)
	projector.Start()
	app.projector = projector

	slog.Info("px initialized", "events", eventsPath, "db", dbPath)
	return nil
}

// cleanupApp shuts down the projector and closes stores.
func cleanupApp() error {
	if app.projector != nil {
		app.projector.Shutdown()
	}
	if app.projStore != nil {
		if err := app.projStore.Close(); err != nil {
			slog.Error("close projection store", "error", err)
		}
	}
	if app.eventStore != nil {
		if err := app.eventStore.Close(); err != nil {
			slog.Error("close event store", "error", err)
		}
	}
	if app.logCleanup != nil {
		app.logCleanup()
	}
	return nil
}

func Execute() error {
	return NewRootCmd().Execute()
}
