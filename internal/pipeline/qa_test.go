package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
)

func TestQAStage_Name(t *testing.T) {
	stage := NewQAStage(git.NewMockRunner())
	if stage.Name() != "qa" {
		t.Errorf("expected name 'qa', got %q", stage.Name())
	}
}

func TestQAStage_GoProject_AllPass(t *testing.T) {
	mock := git.NewMockRunner()
	// git ls-files to detect stack (looks for go.mod)
	mock.AddResponse("go.mod\nmain.go\ninternal/app.go", nil)
	// go vet ./...
	mock.AddResponse("", nil)
	// go test ./... -race
	mock.AddResponse("ok  \tgithub.com/example/pkg\t0.5s", nil)

	stage := NewQAStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed, got %s", result)
	}

	// Verify correct commands for Go project
	if len(mock.Commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(mock.Commands))
	}

	vetCmd := mock.Commands[1]
	if vetCmd.Name != "go" || strings.Join(vetCmd.Args, " ") != "vet ./..." {
		t.Errorf("expected 'go vet ./...', got %s %s", vetCmd.Name, strings.Join(vetCmd.Args, " "))
	}

	testCmd := mock.Commands[2]
	if testCmd.Name != "go" || strings.Join(testCmd.Args, " ") != "test ./... -race" {
		t.Errorf("expected 'go test ./... -race', got %s %s", testCmd.Name, strings.Join(testCmd.Args, " "))
	}
}

func TestQAStage_GoProject_VetFails(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("go.mod\nmain.go", nil)                  // ls-files
	mock.AddResponse("", errors.New("vet: unreachable code")) // go vet fails

	stage := NewQAStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}

func TestQAStage_GoProject_TestFails(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("go.mod\nmain.go", nil)              // ls-files
	mock.AddResponse("", nil)                              // go vet passes
	mock.AddResponse("", errors.New("FAIL: TestFoo"))      // go test fails

	stage := NewQAStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}

func TestQAStage_NodeProject(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("package.json\nsrc/index.ts", nil) // ls-files (node project)
	mock.AddResponse("", nil)                            // npm test

	stage := NewQAStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed, got %s", result)
	}

	// Verify npm test was called
	npmCmd := mock.Commands[1]
	if npmCmd.Name != "npm" || strings.Join(npmCmd.Args, " ") != "test" {
		t.Errorf("expected 'npm test', got %s %s", npmCmd.Name, strings.Join(npmCmd.Args, " "))
	}
}

func TestQAStage_PythonProject(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("requirements.txt\napp.py", nil) // ls-files (python project)
	mock.AddResponse("", nil)                          // pytest

	stage := NewQAStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed, got %s", result)
	}

	pytestCmd := mock.Commands[1]
	if pytestCmd.Name != "pytest" {
		t.Errorf("expected 'pytest', got %s", pytestCmd.Name)
	}
}

func TestQAStage_UnknownStack(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("README.md\nsome-file.txt", nil) // no recognized markers

	stage := NewQAStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unknown stack should still pass (no QA commands to run)
	if result != StagePassed {
		t.Errorf("expected StagePassed for unknown stack, got %s", result)
	}
}

func TestQAStage_LsFilesError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", errors.New("ls-files failed"))

	stage := NewQAStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}
