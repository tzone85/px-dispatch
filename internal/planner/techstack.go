package planner

import (
	"strings"

	"github.com/tzone85/px-dispatch/internal/git"
)

// FormatTechStack formats a git.TechStack into a string suitable for
// inclusion in the planner prompt. Fields that are empty are omitted.
func FormatTechStack(ts git.TechStack) string {
	var parts []string

	if ts.Language != "" {
		parts = append(parts, "Language: "+ts.Language)
	}
	if ts.Framework != "" {
		parts = append(parts, "Framework: "+ts.Framework)
	}
	if ts.TestRunner != "" {
		parts = append(parts, "Test Runner: "+ts.TestRunner)
	}
	if ts.Linter != "" {
		parts = append(parts, "Linter: "+ts.Linter)
	}
	if ts.BuildTool != "" {
		parts = append(parts, "Build Tool: "+ts.BuildTool)
	}
	if ts.PackageManager != "" {
		parts = append(parts, "Package Manager: "+ts.PackageManager)
	}

	return strings.Join(parts, ", ")
}
