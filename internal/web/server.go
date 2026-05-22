package web

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/tzone85/project-x/internal/state"
)

// ServerConfig holds web dashboard configuration.
type ServerConfig struct {
	Port          int
	Bind          string  // default "127.0.0.1"
	Version       string  // px build version, surfaced on /api/about
	DailyLimitUSD float64 // daily cost budget, surfaced on /api/cost
	LogPath       string  // path to px.log; surfaced via /api/logs (empty disables)
	EventStore    state.EventStore
	ProjStore     *state.SQLiteStore
	DB            *sql.DB
}

// Server is the web dashboard HTTP server.
type Server struct {
	config ServerConfig
	server *http.Server
	hub    *SSEHub
}

// NewServer creates a web dashboard server with all routes registered.
// It uses Go 1.22+ method-based routing patterns.
func NewServer(cfg ServerConfig) *Server {
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 7890
	}

	hub := NewSSEHub()

	mux := http.NewServeMux()

	h := &Handlers{
		eventStore:    cfg.EventStore,
		projStore:     cfg.ProjStore,
		db:            cfg.DB,
		version:       cfg.Version,
		dailyLimitUSD: cfg.DailyLimitUSD,
		logPath:       cfg.LogPath,
	}

	// API routes (Go 1.22+ method-based routing).
	mux.HandleFunc("GET /api/requirements", h.ListRequirements)
	mux.HandleFunc("GET /api/stories", h.ListStories)
	mux.HandleFunc("GET /api/agents", h.ListAgents)
	mux.HandleFunc("GET /api/events", h.ListEvents)
	mux.HandleFunc("GET /api/escalations", h.ListEscalations)
	mux.HandleFunc("GET /api/cost", h.GetCost)
	mux.HandleFunc("GET /api/health", h.GetHealth)
	mux.HandleFunc("GET /api/about", h.GetAbout)
	mux.HandleFunc("GET /api/logs", h.GetLogs)
	mux.HandleFunc("GET /api/stream", hub.ServeHTTP)

	// Static files (embedded dashboard assets).
	mux.Handle("GET /", staticHandler())

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)

	s := &Server{
		config: cfg,
		hub:    hub,
		server: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}

	// Print security warning if binding to non-localhost.
	if cfg.Bind != "127.0.0.1" && cfg.Bind != "localhost" {
		slog.Warn("web dashboard binding to non-localhost address",
			"bind", cfg.Bind,
			"warning", "pipeline state and cost data will be accessible from other machines")
	}

	return s
}

// Start begins serving. Blocks until context is cancelled or an error occurs.
func (s *Server) Start(ctx context.Context) error {
	addr := s.server.Addr
	slog.Info("web dashboard starting", "url", fmt.Sprintf("http://%s", addr))

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
	}()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	if err := s.server.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Broadcast sends an event to all connected SSE clients.
func (s *Server) Broadcast(eventType, data string) {
	s.hub.Broadcast(eventType, data)
}
