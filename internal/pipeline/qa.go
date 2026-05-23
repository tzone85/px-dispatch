package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/tzone85/px-dispatch/internal/git"
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
			if specRequiresTests(sc) {
				// If the story's acceptance criteria explicitly require tests,
				// we never tolerate a "no tests" message — that IS the
				// failure mode the spec is describing.
				return StageFailed, fmt.Errorf("%s %s: %w", cmd.name, strings.Join(cmd.args, " "), err)
			}
			if isNoTestsError(cmd.name, err) {
				continue
			}
			return StageFailed, fmt.Errorf("%s %s: %w", cmd.name, strings.Join(cmd.args, " "), err)
		}
	}

	return StagePassed, nil
}

// specRequiresTests inspects the story's acceptance criteria for explicit
// test demands. If the spec calls for tests, the QA stage must NOT swallow
// "no test files" messages — those are real failures (per code-quality
// audit finding #1). Today's heuristic matches common phrasing in our
// requirements; tighten as more cases surface.
func specRequiresTests(sc StoryContext) bool {
	hay := strings.ToLower(sc.AcceptanceCriteria + " " + sc.StoryDescription)
	for _, marker := range []string{
		"test passes", "tests pass", "unit test", "table-driven test",
		"coverage ≥", "coverage >=", "npm test", "go test", "pytest",
		"tdd", "written first",
	} {
		if strings.Contains(hay, marker) {
			return true
		}
	}
	return false
}

// isNoTestsError reports whether the error from a test command means
// "no test files found yet" rather than a real failure. Scoped per-runner to
// avoid masking a jest/vitest exit-1 with a coincidental substring from
// another runner (code-quality audit finding #1).
//
// Note: `go test ./...` returns exit code 0 when packages have no test files
// (it prints `[no test files]` to stdout) — so it never reaches this branch.
// That marker has been removed from the matcher.
func isNoTestsError(runnerName string, err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	var markers []string
	switch runnerName {
	case "npm", "node", "yarn", "pnpm":
		// vitest / jest / mocha — each phrasing.
		markers = []string{
			"No test files found",       // vitest
			"No tests found",            // jest
			"No tests found matching",   // vitest glob miss
			"0 passing",                 // mocha when nothing matches
		}
	case "pytest", "python", "python3":
		markers = []string{
			"no tests ran",
			"no tests collected",
			"collected 0 items",
		}
	}
	for _, marker := range markers {
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
