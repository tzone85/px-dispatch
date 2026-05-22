package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/tzone85/project-x/internal/dashboard"
	"github.com/tzone85/project-x/internal/state"
	"github.com/tzone85/project-x/internal/web"
)

func newDashboardCmd() *cobra.Command {
	var webMode bool
	var port int
	var bind string

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Launch the real-time dashboard",
		Long:  "Displays pipeline status, agent health, costs, and events. Default: TUI mode. Use --web for browser.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if webMode {
				return runWebDashboard(cmd.Context(), port, bind)
			}
			return runTUIDashboard()
		},
	}

	cmd.Flags().BoolVar(&webMode, "web", false, "launch browser-based dashboard instead of TUI")
	cmd.Flags().IntVar(&port, "port", 7890, "port for web dashboard")
	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "bind address for web dashboard")
	return cmd
}

// teaRunner is the function used to actually run the bubbletea program. Tests
// override it to avoid touching the real terminal (which can deadlock under
// -race when stdin/stdout are swapped concurrently with tea reading them).
var teaRunner = func(p *tea.Program) error {
	_, err := p.Run()
	return err
}

func runTUIDashboard() error {
	logPath := filepath.Join(app.stateDir, "logs")

	cfg := dashboard.Config{
		EventStore: app.eventStore,
		ProjStore:  app.projStore,
		DB:         app.projStore.DB(),
		Version:    version,
		ReqFilter:  state.ReqFilter{ExcludeArchived: true},
		LogPath:    logPath,
		DailyLimit: app.config.Budget.MaxCostPerDayUSD,
	}

	model := dashboard.New(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())

	return teaRunner(p)
}

func runWebDashboard(ctx context.Context, port int, bind string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Signal handling for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down web dashboard...")
		cancel()
	}()

	srv := web.NewServer(web.ServerConfig{
		Port:          port,
		Bind:          bind,
		Version:       version,
		DailyLimitUSD: app.config.Budget.MaxCostPerDayUSD,
		LogPath:       filepath.Join(app.stateDir, "logs", "px.log"),
		EventStore:    app.eventStore,
		ProjStore:     app.projStore,
		DB:            app.projStore.DB(),
	})

	url := fmt.Sprintf("http://%s:%d", bind, port)
	fmt.Printf("Web dashboard: %s\n", url)
	fmt.Println("Press Ctrl+C to stop")

	// Best-effort browser open — failure is silently ignored.
	openBrowser(url)

	return srv.Start(ctx)
}

// browserGOOS reports the current operating system identifier. It is a
// variable so tests can simulate non-darwin platforms without having to
// cross-build.
var browserGOOS = func() string { return runtime.GOOS }

// openBrowser opens the URL in the default browser. Best-effort; no error on failure.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch browserGOOS() {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start() // fire and forget
}
