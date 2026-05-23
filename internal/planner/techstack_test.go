package planner

import (
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
)

func TestFormatTechStack_AllFields(t *testing.T) {
	ts := git.TechStack{
		Language:       "go",
		Framework:      "cobra",
		TestRunner:     "go test",
		Linter:         "golangci-lint",
		BuildTool:      "go build",
		PackageManager: "go modules",
	}
	result := FormatTechStack(ts)
	expected := "Language: go, Framework: cobra, Test Runner: go test, Linter: golangci-lint, Build Tool: go build, Package Manager: go modules"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatTechStack_PartialFields(t *testing.T) {
	ts := git.TechStack{
		Language:   "python",
		TestRunner: "pytest",
	}
	result := FormatTechStack(ts)
	expected := "Language: python, Test Runner: pytest"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatTechStack_EmptyStruct(t *testing.T) {
	ts := git.TechStack{}
	result := FormatTechStack(ts)
	if result != "" {
		t.Errorf("expected empty string for empty tech stack, got %q", result)
	}
}

func TestFormatTechStack_LanguageOnly(t *testing.T) {
	ts := git.TechStack{Language: "rust"}
	result := FormatTechStack(ts)
	if result != "Language: rust" {
		t.Errorf("expected 'Language: rust', got %q", result)
	}
}
