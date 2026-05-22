package dashboard

import (
	"strings"
	"testing"
)

func TestStatusColor(t *testing.T) {
	tests := []struct {
		status string
		want   string // hex color value
	}{
		{"active", "#00FF00"},
		{"in_progress", "#00FF00"},
		{"merged", "#00FF00"},
		{"idle", "#888888"},
		{"planned", "#888888"},
		{"draft", "#888888"},
		{"stuck", "#FF0000"},
		{"qa_failed", "#FF0000"},
		{"pending", "#FF0000"},
		{"assigned", "#5588FF"},
		{"review", "#FFFF00"},
		{"pr_submitted", "#FFFF00"},
		{"qa", "#FF00FF"},
		{"paused", "#FF8800"},
		{"anything-else", "#FFFFFF"},
		{"", "#FFFFFF"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := string(statusColor(tt.status))
			if got != tt.want {
				t.Errorf("statusColor(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestStatusBadge_ContainsStatus(t *testing.T) {
	out := statusBadge("merged")
	if !strings.Contains(out, "merged") {
		t.Errorf("expected badge to contain status text, got %q", out)
	}
	if !strings.Contains(out, "[") || !strings.Contains(out, "]") {
		t.Errorf("expected brackets around status, got %q", out)
	}
}

func TestComplexityBadge_ColorThresholds(t *testing.T) {
	// We can't easily assert color codes in lipgloss output without ANSI parsing,
	// but we can confirm the formatted text is correct for all thresholds.
	for _, c := range []int{0, 1, 4, 5, 7, 8, 10} {
		out := complexityBadge(c)
		if !strings.Contains(out, formatComplexity(c)) {
			t.Errorf("complexityBadge(%d) missing %q in %q", c, formatComplexity(c), out)
		}
	}
}

func TestFormatComplexity(t *testing.T) {
	if got := formatComplexity(0); got != "[C0]" {
		t.Errorf("formatComplexity(0) = %q, want [C0]", got)
	}
	if got := formatComplexity(13); got != "[C13]" {
		t.Errorf("formatComplexity(13) = %q, want [C13]", got)
	}
}
