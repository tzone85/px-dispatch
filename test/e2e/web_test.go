package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tzone85/px-dispatch/internal/state"
	"github.com/tzone85/px-dispatch/internal/web"
)

// freePort returns an OS-assigned free TCP port for binding the test server.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return port
}

// waitFor200 polls url until it returns HTTP 200 or the timeout elapses.
func waitFor200(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("server did not become ready: %s", url)
}

// TestWebAPI_EndToEnd boots the real web.Server in-process and round-trips
// every documented endpoint. This is the wiring guard: if a handler is added
// without being registered (or vice-versa), this test fails.
func TestWebAPI_EndToEnd(t *testing.T) {
	es, ps := setupTestStores(t)

	dir := t.TempDir()
	logPath := filepath.Join(dir, "px.log")

	port := freePort(t)
	srv := web.NewServer(web.ServerConfig{
		Port:          port,
		Bind:          "127.0.0.1",
		Version:       "test-1.2.3",
		DailyLimitUSD: 12.34,
		LogPath:       logPath,
		EventStore:    es,
		ProjStore:     ps,
		DB:            ps.DB(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	waitFor200(t, base+"/api/health", 2*time.Second)

	t.Run("about reflects ServerConfig.Version", func(t *testing.T) {
		body := getJSON(t, base+"/api/about")
		if got, want := body["name"], "px-dispatch"; got != want {
			t.Errorf("name: got %v, want %v", got, want)
		}
		if got, want := body["version"], "test-1.2.3"; got != want {
			t.Errorf("version: got %v, want %v", got, want)
		}
		stages, ok := body["pipeline_stages"].([]any)
		if !ok || len(stages) != 7 {
			t.Errorf("pipeline_stages: got %v, want 7-element array", body["pipeline_stages"])
		}
	})

	t.Run("cost surfaces daily_limit_usd from config", func(t *testing.T) {
		body := getJSON(t, base+"/api/cost")
		if got := body["daily_limit_usd"]; got != 12.34 {
			t.Errorf("daily_limit_usd: got %v, want 12.34", got)
		}
	})

	t.Run("escalations returns empty array (not null)", func(t *testing.T) {
		raw := getRaw(t, base+"/api/escalations")
		if raw != "[]\n" && raw != "[]" {
			t.Errorf("expected empty array body, got %q", raw)
		}
	})

	t.Run("logs returns empty when file missing", func(t *testing.T) {
		body := getJSON(t, base+"/api/logs")
		lines, _ := body["lines"].([]any)
		if len(lines) != 0 {
			t.Errorf("expected 0 lines, got %d", len(lines))
		}
		if body["path"] != logPath {
			t.Errorf("path: got %v, want %v", body["path"], logPath)
		}
	})

	t.Run("requirements/stories/agents/events all 200", func(t *testing.T) {
		for _, ep := range []string{
			"/api/requirements",
			"/api/stories",
			"/api/agents",
			"/api/events",
		} {
			resp, err := http.Get(base + ep)
			if err != nil {
				t.Fatalf("GET %s: %v", ep, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s: status %d", ep, resp.StatusCode)
			}
		}
	})

	// Project an escalation event and confirm /api/escalations now returns it.
	t.Run("escalations round-trip via projector", func(t *testing.T) {
		evt := state.NewEvent(state.EventEscalationCreated, "junior-a1", "story-1", map[string]any{
			"id":         "esc-1",
			"story_id":   "story-1",
			"from_agent": "junior-a1",
			"reason":     "blocked",
		})
		if err := ps.Project(evt); err != nil {
			t.Fatalf("project escalation: %v", err)
		}

		body := getJSON(t, base+"/api/escalations")
		arr, ok := body["__arr__"].([]any)
		if !ok {
			t.Fatalf("expected array result, got %v", body)
		}
		if len(arr) != 1 {
			t.Fatalf("expected 1 escalation, got %d", len(arr))
		}
	})

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("server exited with: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down within 2s")
	}
}

// getJSON GETs url and decodes the body. If the top-level JSON is an array,
// it is wrapped as {"__arr__": [...]} so the caller can use a single helper.
func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}

	var anyVal any
	if err := json.Unmarshal(body, &anyVal); err != nil {
		t.Fatalf("decode %s: %v (raw: %s)", url, err, string(body))
	}

	switch v := anyVal.(type) {
	case map[string]any:
		return v
	case []any:
		return map[string]any{"__arr__": v}
	default:
		t.Fatalf("unexpected JSON top-level type from %s: %T", url, anyVal)
		return nil
	}
}

// getRaw GETs url and returns the raw response body as a string.
func getRaw(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return string(body)
}
