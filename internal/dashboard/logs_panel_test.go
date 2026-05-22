package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestColorizeLogLine(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"error", "ERROR something bad"},
		{"warn", "WARN heads up"},
		{"debug", "DEBUG noisy"},
		{"info default", "INFO normal"},
		{"plain", "no level"},
	}
	for _, tt := range tests {
		out := colorizeLogLine(tt.line)
		if !strings.Contains(out, tt.line) {
			t.Errorf("colorizeLogLine(%q) should contain original text, got %q", tt.line, out)
		}
	}
}

func TestTailFile_LessThanN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	lines, err := tailFile(path, 10)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	if len(lines) != 3 {
		t.Errorf("want 3 lines, got %d", len(lines))
	}
}

func TestTailFile_MoreThanN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte("1\n2\n3\n4\n5\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	lines, err := tailFile(path, 2)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	if len(lines) != 2 {
		t.Errorf("want 2 lines, got %d", len(lines))
	}
	if lines[0] != "4" || lines[1] != "5" {
		t.Errorf("want last 2 lines [4 5], got %v", lines)
	}
}

func TestTailFile_NotFound(t *testing.T) {
	if _, err := tailFile("/no/such/file/here", 10); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestTailFile_ScannerErrorOnHugeLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	// Write a single line larger than the 64KB scanner buffer.
	huge := strings.Repeat("X", 128*1024)
	if err := os.WriteFile(path, []byte(huge), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := tailFile(path, 10); err == nil {
		t.Error("expected scanner buffer error for oversized line")
	}
}

func TestRenderLogs_EmptyPath(t *testing.T) {
	out := renderLogs("")
	if !strings.Contains(out, "No log file configured") {
		t.Errorf("expected placeholder for empty path, got %q", out)
	}
}

func TestRenderLogs_CannotRead(t *testing.T) {
	out := renderLogs("/no/such/path/exists/maybe")
	if !strings.Contains(out, "Cannot read log file") {
		t.Errorf("expected read-error placeholder, got %q", out)
	}
}

func TestRenderLogs_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := renderLogs(path)
	if !strings.Contains(out, "Log file is empty") {
		t.Errorf("expected empty placeholder, got %q", out)
	}
}

func TestRenderLogs_WithLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte("INFO ok\nERROR boom\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := renderLogs(path)
	if !strings.Contains(out, "showing last 2 lines") {
		t.Errorf("expected count header, got %q", out)
	}
	if !strings.Contains(out, "INFO ok") || !strings.Contains(out, "ERROR boom") {
		t.Errorf("expected both lines, got %q", out)
	}
}
