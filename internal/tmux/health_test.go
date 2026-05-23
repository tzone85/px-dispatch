package tmux

import (
	"fmt"
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
)

func TestHealth_Healthy(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                 // has-session succeeds
	mock.AddResponse("12345 0 0", nil)        // list-panes: pid, not dead, exit 0
	mock.AddResponse("some output here", nil) // capture-pane

	result := SessionHealth(mock, "test", "")
	if result.Status != Healthy {
		t.Errorf("expected Healthy, got %s", result.Status)
	}
	if result.OutputHash == "" {
		t.Error("expected non-empty output hash")
	}
	if result.PanePID != "12345" {
		t.Errorf("expected PanePID 12345, got %s", result.PanePID)
	}
}

func TestHealth_Dead(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)          // has-session succeeds
	mock.AddResponse("12345 1 1", nil) // pane_dead = 1

	result := SessionHealth(mock, "test", "")
	if result.Status != Dead {
		t.Errorf("expected Dead, got %s", result.Status)
	}
	if result.PanePID != "12345" {
		t.Errorf("expected PanePID 12345, got %s", result.PanePID)
	}
}

func TestHealth_Missing(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session")) // has-session fails

	result := SessionHealth(mock, "test", "")
	if result.Status != Missing {
		t.Errorf("expected Missing, got %s", result.Status)
	}
}

func TestHealth_Stale(t *testing.T) {
	// First call to establish baseline
	mock1 := git.NewMockRunner()
	mock1.AddResponse("", nil)                 // has-session
	mock1.AddResponse("12345 0 0", nil)        // alive
	mock1.AddResponse("same output", nil)      // capture-pane

	result1 := SessionHealth(mock1, "test", "")
	if result1.Status != Healthy {
		t.Fatalf("expected first check to be Healthy, got %s", result1.Status)
	}

	// Second call with same output hash — should detect staleness
	mock2 := git.NewMockRunner()
	mock2.AddResponse("", nil)            // has-session
	mock2.AddResponse("12345 0 0", nil)   // alive
	mock2.AddResponse("same output", nil) // same output

	result2 := SessionHealth(mock2, "test", result1.OutputHash)
	if result2.Status != Stale {
		t.Errorf("expected Stale when output unchanged, got %s", result2.Status)
	}
}

func TestHealth_NotStaleWithDifferentOutput(t *testing.T) {
	mock1 := git.NewMockRunner()
	mock1.AddResponse("", nil)
	mock1.AddResponse("12345 0 0", nil)
	mock1.AddResponse("output version 1", nil)

	result1 := SessionHealth(mock1, "test", "")

	mock2 := git.NewMockRunner()
	mock2.AddResponse("", nil)
	mock2.AddResponse("12345 0 0", nil)
	mock2.AddResponse("output version 2", nil) // different output

	result2 := SessionHealth(mock2, "test", result1.OutputHash)
	if result2.Status != Healthy {
		t.Errorf("expected Healthy when output changed, got %s", result2.Status)
	}
}

func TestHealth_ListPanesError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                         // has-session succeeds
	mock.AddResponse("", fmt.Errorf("pane error"))    // list-panes fails

	result := SessionHealth(mock, "test", "")
	if result.Status != Missing {
		t.Errorf("expected Missing on pane error, got %s", result.Status)
	}
}

func TestHealth_CaptureError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)              // has-session
	mock.AddResponse("12345 0 0", nil)     // list-panes
	mock.AddResponse("", fmt.Errorf("capture failed")) // capture-pane fails

	result := SessionHealth(mock, "test", "")
	// Should still be Healthy, just can't determine staleness
	if result.Status != Healthy {
		t.Errorf("expected Healthy on capture error, got %s", result.Status)
	}
	if result.PanePID != "12345" {
		t.Errorf("expected PanePID 12345, got %s", result.PanePID)
	}
}

func TestHashOutput_Deterministic(t *testing.T) {
	h1 := hashOutput("test input")
	h2 := hashOutput("test input")

	if h1 != h2 {
		t.Errorf("expected same hash for same input, got %s and %s", h1, h2)
	}
}

func TestHashOutput_DifferentInputs(t *testing.T) {
	h1 := hashOutput("input A")
	h2 := hashOutput("input B")

	if h1 == h2 {
		t.Error("expected different hashes for different inputs")
	}
}
