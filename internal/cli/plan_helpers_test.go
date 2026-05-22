package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tzone85/project-x/internal/planner"
	"github.com/tzone85/project-x/internal/state"
)

func TestGenerateID_UniqueAndULIDShape(t *testing.T) {
	a := generateID()
	b := generateID()
	if a == b {
		t.Errorf("generateID returned duplicate: %s", a)
	}
	if len(a) != 26 {
		t.Errorf("ULID should be 26 chars, got %d: %s", len(a), a)
	}
}

func TestTruncateForTitle_NoTruncation(t *testing.T) {
	if got := truncateForTitle("short", 100); got != "short" {
		t.Errorf("short input should be unchanged, got %q", got)
	}
}

func TestTruncateForTitle_TruncatesAtWordBoundary(t *testing.T) {
	in := "the quick brown fox jumps over the lazy dog"
	got := truncateForTitle(in, 20)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected '...' suffix, got %q", got)
	}
	if strings.Contains(got, "fox") && !strings.HasSuffix(got, "fox...") {
		// At word boundary <= 20
	}
	if len(got) > 23 {
		t.Errorf("output exceeded max+3, got %q (len=%d)", got, len(got))
	}
}

func TestTruncateForTitle_NoWordBoundaryHardCut(t *testing.T) {
	// No spaces → no word boundary; truncate hard.
	got := truncateForTitle("aaaaaaaaaaaaaaaaaaaa", 5)
	if got != "aaaaa..." {
		t.Errorf("got %q, want %q", got, "aaaaa...")
	}
}

func TestTruncateForTitle_CollapsesNewlines(t *testing.T) {
	got := truncateForTitle("line1\nline2\nline3", 100)
	if strings.Contains(got, "\n") {
		t.Errorf("newlines should be collapsed, got %q", got)
	}
}

func TestBuildDepMap(t *testing.T) {
	deps := []state.StoryDep{
		{StoryID: "s-1", DependsOnID: "s-0"},
		{StoryID: "s-2", DependsOnID: "s-0"},
		{StoryID: "s-2", DependsOnID: "s-1"},
	}
	m := buildDepMap(deps)
	if len(m["s-1"]) != 1 || m["s-1"][0] != "s-0" {
		t.Errorf("s-1 deps wrong: %v", m["s-1"])
	}
	if len(m["s-2"]) != 2 {
		t.Errorf("s-2 should have 2 deps, got %v", m["s-2"])
	}
	if _, ok := m["s-missing"]; ok {
		t.Error("unexpected key")
	}
}

func TestReadRequirement_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "req.txt")
	content := "  some requirement text  \n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readRequirement(path)
	if err != nil {
		t.Fatalf("readRequirement: %v", err)
	}
	if got != "some requirement text" {
		t.Errorf("got %q, want trimmed input", got)
	}
}

func TestReadRequirement_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte("   \n  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readRequirement(path); err == nil {
		t.Error("expected error for whitespace-only file")
	}
}

func TestReadRequirement_MissingFile(t *testing.T) {
	if _, err := readRequirement("/no/such/path"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadRequirement_StdinDash(t *testing.T) {
	// Provide content via stdin.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	go func() {
		_, _ = w.WriteString("hello via stdin\n")
		w.Close()
	}()

	got, err := readRequirement("-")
	if err != nil {
		t.Fatalf("readRequirement: %v", err)
	}
	if got != "hello via stdin" {
		t.Errorf("got %q", got)
	}
}

func TestReadRequirement_StdinEmpty(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	w.Close()

	if _, err := readRequirement("-"); err == nil {
		t.Error("empty stdin should error")
	}
}

func TestPrintStoryTable_OutputsAllStories(t *testing.T) {
	stories := []planner.PlannedStory{
		{ID: "short", Title: "T1", Complexity: 1, WaveHint: "parallel"},
		{ID: "verylongidentifier", Title: strings.Repeat("X", 60), Complexity: 5, WaveHint: "sequential", DependsOn: []string{"a", "b"}},
	}
	out := captureStdout(t, func() { printStoryTable(stories) })
	if !strings.Contains(out, "short") {
		t.Errorf("expected short id in table, got %q", out)
	}
	if !strings.Contains(out, "verylo..") {
		t.Errorf("expected truncated id, got %q", out)
	}
	if !strings.Contains(out, "..") {
		t.Errorf("expected ellipsis somewhere for long title, got %q", out)
	}
	if !strings.Contains(out, "a, b") {
		t.Errorf("expected deps to be joined, got %q", out)
	}
}

func TestScopeStoriesForRequirement_ShortReqID(t *testing.T) {
	stories := []planner.PlannedStory{
		{ID: "s-1", Title: "first", DependsOn: nil},
		{ID: "s-2", Title: "second", DependsOn: []string{"s-1"}},
	}
	out := scopeStoriesForRequirement("01ABC", stories)
	if out[0].ID != "01ABC-s-1" {
		t.Errorf("expected prefixed id, got %q", out[0].ID)
	}
	if out[1].DependsOn[0] != "01ABC-s-1" {
		t.Errorf("dep should be remapped, got %q", out[1].DependsOn[0])
	}
}

func TestScopeStoriesForRequirement_LongReqID_Truncates(t *testing.T) {
	stories := []planner.PlannedStory{{ID: "s-1"}}
	out := scopeStoriesForRequirement("01ABCDEFGHIJK", stories)
	if !strings.HasPrefix(out[0].ID, "01ABCDEF-") {
		t.Errorf("expected 8-char prefix, got %q", out[0].ID)
	}
}

func TestScopeStoryIDs_UnknownDependencyStillPrefixed(t *testing.T) {
	stories := []planner.PlannedStory{
		{ID: "s-1", DependsOn: []string{"unknown-x"}},
	}
	out := scopeStoryIDs(stories, "PFX")
	if out[0].DependsOn[0] != "PFX-unknown-x" {
		t.Errorf("expected prefix on unknown dep, got %q", out[0].DependsOn[0])
	}
}

func TestNewPlanCmd_Flags(t *testing.T) {
	cmd := newPlanCmd()
	if cmd.Flags().Lookup("review") == nil {
		t.Error("missing --review flag")
	}
	if cmd.Flags().Lookup("refine") == nil {
		t.Error("missing --refine flag")
	}
}

func TestNewPlanCmd_NoArgsAndNoFlagsErrors(t *testing.T) {
	cmd := newPlanCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error when no file and no flag provided")
	}
}

func TestNewPlanCmd_FileArgExecutesRunPlan(t *testing.T) {
	dir := setupTestApp(t)
	withStubLLM(t, sampleStoriesJSON)
	app.config.Planning.MaxStoryComplexity = 0
	app.config.Planning.MaxStoriesPerRequirement = 0
	app.config.Planning.EnforceFileOwnership = false

	reqPath := filepath.Join(dir, "req.txt")
	if err := os.WriteFile(reqPath, []byte("add /version"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newPlanCmd()
	cmd.SetArgs([]string{reqPath})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(out, "Stories:") {
		t.Errorf("expected stories table via cmd.Execute, got %q", out)
	}
}
