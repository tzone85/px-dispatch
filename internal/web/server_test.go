package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/tzone85/px-dispatch/internal/state"
)

// findFreePort returns a port that is currently not in use.
func findFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestNewServer_DefaultConfig(t *testing.T) {
	projStore, err := state.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { projStore.Close() })

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventStore, err := state.NewFileStore(eventsPath)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	t.Cleanup(func() { eventStore.Close() })

	srv := NewServer(ServerConfig{
		EventStore: eventStore,
		ProjStore:  projStore,
		DB:         projStore.DB(),
	})

	if srv.server.Addr != "127.0.0.1:7890" {
		t.Errorf("default addr: got %q, want %q", srv.server.Addr, "127.0.0.1:7890")
	}
}

func TestNewServer_CustomConfig(t *testing.T) {
	projStore, err := state.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { projStore.Close() })

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventStore, err := state.NewFileStore(eventsPath)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	t.Cleanup(func() { eventStore.Close() })

	srv := NewServer(ServerConfig{
		Port:       9999,
		Bind:       "0.0.0.0",
		EventStore: eventStore,
		ProjStore:  projStore,
		DB:         projStore.DB(),
	})

	if srv.server.Addr != "0.0.0.0:9999" {
		t.Errorf("custom addr: got %q, want %q", srv.server.Addr, "0.0.0.0:9999")
	}
}

func TestServer_StartAndShutdown(t *testing.T) {
	projStore, err := state.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { projStore.Close() })

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventStore, err := state.NewFileStore(eventsPath)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	t.Cleanup(func() { eventStore.Close() })

	port := findFreePort(t)

	srv := NewServer(ServerConfig{
		Port:       port,
		Bind:       "127.0.0.1",
		EventStore: eventStore,
		ProjStore:  projStore,
		DB:         projStore.DB(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Wait for server to be ready.
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(addr + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Verify health endpoint is reachable.
	resp, err := http.Get(addr + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status: got %d, want 200", resp.StatusCode)
	}

	var health healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("health.Status: got %q, want %q", health.Status, "ok")
	}

	// Verify requirements endpoint returns empty array.
	resp2, err := http.Get(addr + "/api/requirements")
	if err != nil {
		t.Fatalf("GET /api/requirements: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("requirements status: got %d, want 200", resp2.StatusCode)
	}

	// Shutdown.
	cancel()
	if err := <-errCh; err != nil {
		t.Errorf("Start returned error: %v", err)
	}
}

func TestServer_Broadcast(t *testing.T) {
	projStore, err := state.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { projStore.Close() })

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventStore, err := state.NewFileStore(eventsPath)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	t.Cleanup(func() { eventStore.Close() })

	srv := NewServer(ServerConfig{
		Port:       findFreePort(t),
		Bind:       "127.0.0.1",
		EventStore: eventStore,
		ProjStore:  projStore,
		DB:         projStore.DB(),
	})

	// Broadcast should not panic with no clients.
	srv.Broadcast("test", `{"msg":"hello"}`)

	if srv.hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", srv.hub.ClientCount())
	}
}
