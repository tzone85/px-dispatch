package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/tzone85/project-x/internal/git"
)

// qaCommand describes a single QA check to run.
type qaCommand struct {
	name string
	args []string
}

// QAStage detects the project's tech stack and runs appropriate QA commands
// (linting, testing). If any command fails, the stage fails.
type QAStage struct {
	runner git.CommandRunner
}

// NewQAStage creates a QAStage that uses the given runner.
func NewQAStage(runner git.CommandRunner) *QAStage {
	return &QAStage{runner: runner}
}

// Name returns the stage identifier.
func (s *QAStage) Name() string { return "qa" }

// Execute detects the tech stack and runs the appropriate QA commands.
//
// "No test files found" is treated as a non-failure: an early scaffolding
// story may legitimately have no tests yet, and the spec-compliance check
// in the review stage already gates whether tests were *expected* for this
// story. Failing here on absent tests would cause an infinite respawn on
// stories whose acceptance criteria don't include tests.
func (s *QAStage) Execute(_ context.Context, sc StoryContext) (StageResult, error) {
	files, err := s.runner.Run(sc.WorktreePath, "git", "ls-files")
	if err != nil {
		return StageFailed, fmt.Errorf("listing files: %w", err)
	}

	commands := detectQACommands(files)
	if len(commands) == 0 {
		return StagePassed, nil
	}

	for _, cmd := range commands {
		if _, err := s.runner.Run(sc.WorktreePath, cmd.name, cmd.args...); err != nil {
			if isNoTestsError(err) {
				continue
			}
			return StageFailed, fmt.Errorf("%s %s: %w", cmd.name, strings.Join(cmd.args, " "), err)
		}
	}

	return StagePassed, nil
}

// isNoTestsError reports whether the error from a test command means
// "no test files found yet" rather than a real failure. Recognised today:
//   - vitest:  "No test files found, exiting with code 1"
//   - jest:    "No tests found"
//   - pytest:  "no tests ran in"
//   - go test: "[no test files]"
func isNoTestsError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, marker := range []string{
		"No test files found",
		"No tests found",
		"no tests ran",
		"[no test files]",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// detectQACommands determines which QA commands to run based on file markers.
func detectQACommands(fileList string) []qaCommand {
	files := strings.Split(fileList, "\n")
	markers := make(map[string]bool, len(files))
	for _, f := range files {
		markers[strings.TrimSpace(f)] = true
	}

	switch {
	case markers["go.mod"]:
		return []qaCommand{
			{name: "go", args: []string{"vet", "./..."}},
			{name: "go", args: []string{"test", "./...", "-race"}},
		}
	case markers["package.json"]:
		return []qaCommand{
			{name: "npm", args: []string{"test"}},
		}
	case markers["requirements.txt"], markers["pyproject.toml"]:
		return []qaCommand{
			{name: "pytest"},
		}
	default:
		return nil
	}
}
