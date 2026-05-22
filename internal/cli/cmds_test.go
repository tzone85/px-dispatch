package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tzone85/project-x/internal/state"
)

// --- agents ------------------------------------------------------------------

func TestAgentsCmd_Empty(t *testing.T) {
	setupTestApp(t)
	out := captureStdout(t, func() {
		cmd := newAgentsCmd()
		cmd.SetArgs([]string{})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(out, "No agents found") {
		t.Errorf("expected empty placeholder, got %q", out)
	}
}

func TestAgentsCmd_WithAgent(t *testing.T) {
	setupTestApp(t)
	// Long IDs to exercise truncation branches in newAgentsCmd.
	evt := state.NewEvent(state.EventAgentSpawned, "AGENT-LONG-ID-XYZ", "STORY-LONG-ID-XYZ", map[string]any{
		"id":           "AGENT-VERY-LONG-IDENTIFIER",
		"type":         "junior",
		"model":        "claude-sonnet",
		"runtime":      "claude-code",
		"session_name": "px-story-name",
		"story_id":     "STORY-VERY-LONG-IDENTIFIER",
	})
	if err := app.projStore.Project(evt); err != nil {
		t.Fatalf("project: %v", err)
	}

	out := captureStdout(t, func() {
		_ = newAgentsCmd().Execute()
	})
	if !strings.Contains(out, "AGENT-VERY-L..") {
		t.Errorf("expected truncated agent id, got %q", out)
	}
	if !strings.Contains(out, "STORY-VERY-L..") {
		t.Errorf("expected truncated story id, got %q", out)
	}
}

func TestAgentsCmd_AgentNoStoryNoSession(t *testing.T) {
	setupTestApp(t)
	evt := state.NewEvent(state.EventAgentSpawned, "A2", "", map[string]any{
		"id":           "A2",
		"type":         "junior",
		"model":        "claude",
		"runtime":      "claude-code",
		"session_name": "",
		"story_id":     "",
	})
	_ = app.projStore.Project(evt)

	out := captureStdout(t, func() {
		_ = newAgentsCmd().Execute()
	})
	// "-" placeholder appears for both empty fields.
	if !strings.Contains(out, "-") {
		t.Errorf("expected '-' placeholder, got %q", out)
	}
}

// --- archive -----------------------------------------------------------------

func TestArchiveCmd_RequiresArgs(t *testing.T) {
	cmd := newArchiveCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error without required req-id arg")
	}
}

func TestArchiveCmd_UnknownRequirement_SucceedsSilently(t *testing.T) {
	setupTestApp(t)
	// ArchiveRequirement on a non-existent ID is a no-op success — it just sets
	// the status to archived if the row exists. The command should still print
	// the confirmation.
	out := captureStdout(t, func() {
		cmd := newArchiveCmd()
		cmd.SetArgs([]string{"NOPE"})
		_ = cmd.Execute()
	})
	if !strings.Contains(out, "Archived requirement NOPE") {
		t.Errorf("expected confirmation, got %q", out)
	}
}

func TestArchiveCmd_Success(t *testing.T) {
	setupTestApp(t)
	// Seed a requirement so the archive succeeds.
	evt := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "REQ1", "title": "T", "description": "D", "repo_path": ".",
	})
	if err := app.projStore.Project(evt); err != nil {
		t.Fatalf("project: %v", err)
	}

	out := captureStdout(t, func() {
		cmd := newArchiveCmd()
		cmd.SetArgs([]string{"REQ1"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("archive: %v", err)
		}
	})
	if !strings.Contains(out, "Archived requirement REQ1") {
		t.Errorf("expected archived confirmation, got %q", out)
	}
}

// --- events ------------------------------------------------------------------

func TestEventsCmd_Empty(t *testing.T) {
	setupTestApp(t)
	out := captureStdout(t, func() {
		_ = newEventsCmd().Execute()
	})
	if !strings.Contains(out, "No events found") {
		t.Errorf("expected empty placeholder, got %q", out)
	}
}

func TestEventsCmd_WithEntry(t *testing.T) {
	setupTestApp(t)
	evt := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{"id": "X"})
	if err := app.eventStore.Append(evt); err != nil {
		t.Fatalf("append: %v", err)
	}
	out := captureStdout(t, func() {
		_ = newEventsCmd().Execute()
	})
	if !strings.Contains(out, "req.submitted") {
		t.Errorf("expected event type in output, got %q", out)
	}
}

// --- migrate -----------------------------------------------------------------

func TestMigrateCmd_PrintsState(t *testing.T) {
	setupTestApp(t)
	out := captureStdout(t, func() {
		_ = newMigrateCmd().Execute()
	})
	if !strings.Contains(out, "Database is up to date") {
		t.Errorf("expected migrate confirmation, got %q", out)
	}
}

// --- config ------------------------------------------------------------------

func TestConfigShowCmd(t *testing.T) {
	setupTestApp(t)
	out := captureStdout(t, func() {
		_ = newConfigShowCmd().Execute()
	})
	if !strings.Contains(out, "workspace:") {
		t.Errorf("expected YAML config, got %q", out)
	}
}

func TestConfigValidateCmd_Valid(t *testing.T) {
	setupTestApp(t)
	out := captureStdout(t, func() {
		_ = newConfigValidateCmd().Execute()
	})
	if !strings.Contains(out, "Configuration is valid") {
		t.Errorf("expected validation success, got %q", out)
	}
}

func TestConfigValidateCmd_Invalid(t *testing.T) {
	setupTestApp(t)
	app.config.Workspace.LogLevel = "garbage"
	if err := newConfigValidateCmd().Execute(); err == nil {
		t.Error("expected validation error")
	}
}

func TestConfigCmd_NoArgsShowsHelp(t *testing.T) {
	cmd := newConfigCmd()
	cmd.SetOut(os.Stderr) // Run prints help — just confirm no panic / no nil cmd.
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		// Help may write to stderr and return nil; tolerate either.
		t.Logf("config no-args returned %v (acceptable)", err)
	}
}

// --- status ------------------------------------------------------------------

func TestStatusCmd_NoRequirements(t *testing.T) {
	setupTestApp(t)
	out := captureStdout(t, func() {
		_ = newStatusCmd().Execute()
	})
	if !strings.Contains(out, "No requirements found") {
		t.Errorf("expected empty placeholder, got %q", out)
	}
}

func TestStatusCmd_MultiStatusCommaSeparator(t *testing.T) {
	setupTestApp(t)
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-MS", "title": "multi-status", "description": "d", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}
	// Two stories with different statuses so the count list has a comma between them.
	for _, sid := range []string{"S-A", "S-B"} {
		sEvt := state.NewEvent(state.EventStoryCreated, "planner", sid, map[string]any{
			"id": sid, "req_id": "R-MS", "title": "t", "description": "d",
			"acceptance_criteria": "a", "complexity": 1, "owned_files": []string{},
			"wave_hint": "parallel", "depends_on": []string{},
		})
		_ = app.projStore.Project(sEvt)
	}
	// Mark S-B as merged so we have two distinct statuses (planned + merged).
	m := state.NewEvent(state.EventStoryMerged, "monitor", "S-B", map[string]any{})
	_ = app.projStore.Project(m)

	out := captureStdout(t, func() {
		_ = newStatusCmd().Execute()
	})
	// Both statuses should appear in the count breakdown, comma-separated.
	if !strings.Contains(out, ":") || !strings.Contains(out, ",") {
		t.Errorf("expected comma-separated status counts, got %q", out)
	}
}

func TestStatusCmd_WithRequirementsAndStories(t *testing.T) {
	setupTestApp(t)
	// Use Project so the projection store updates without going through Append.
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R1", "title": "T1", "description": "D", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project req: %v", err)
	}
	s := state.NewEvent(state.EventStoryCreated, "planner", "S1", map[string]any{
		"id": "S1", "req_id": "R1", "title": "story", "description": "d",
		"acceptance_criteria": "a", "complexity": 1, "owned_files": []string{},
		"wave_hint": "parallel", "depends_on": []string{},
	})
	if err := app.projStore.Project(s); err != nil {
		t.Fatalf("project story: %v", err)
	}

	out := captureStdout(t, func() {
		_ = newStatusCmd().Execute()
	})
	if !strings.Contains(out, "R1") {
		t.Errorf("expected R1 in status output, got %q", out)
	}
}

func TestStatusCmd_SpecificRequirementDetail(t *testing.T) {
	setupTestApp(t)
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-D", "title": "Detail", "description": "desc", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}
	out := captureStdout(t, func() {
		cmd := newStatusCmd()
		cmd.SetArgs([]string{"R-D"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(out, "Title:") || !strings.Contains(out, "Detail") {
		t.Errorf("expected detail block, got %q", out)
	}
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected '(none)' for empty stories, got %q", out)
	}
}

func TestStatusCmd_RequirementWithStories(t *testing.T) {
	setupTestApp(t)
	rEvt := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-FULL", "title": "with stories", "description": "d", "repo_path": ".",
	})
	if err := app.projStore.Project(rEvt); err != nil {
		t.Fatalf("project req: %v", err)
	}
	sEvt := state.NewEvent(state.EventStoryCreated, "planner", "S-FULL", map[string]any{
		"id": "S-FULL", "req_id": "R-FULL", "title": "story title", "description": "d",
		"acceptance_criteria": "a", "complexity": 3, "owned_files": []string{},
		"wave_hint": "parallel", "depends_on": []string{},
	})
	if err := app.projStore.Project(sEvt); err != nil {
		t.Fatalf("project story: %v", err)
	}

	out := captureStdout(t, func() {
		cmd := newStatusCmd()
		cmd.SetArgs([]string{"R-FULL"})
		_ = cmd.Execute()
	})
	if !strings.Contains(out, "S-FULL") {
		t.Errorf("expected story id in detail output, got %q", out)
	}
}

func TestStatusCmd_DescriptionShownWhenPresent(t *testing.T) {
	setupTestApp(t)
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-DESC", "title": "with desc", "description": "long description text", "repo_path": ".",
	})
	_ = app.projStore.Project(r)
	out := captureStdout(t, func() {
		cmd := newStatusCmd()
		cmd.SetArgs([]string{"R-DESC"})
		_ = cmd.Execute()
	})
	if !strings.Contains(out, "long description text") {
		t.Errorf("expected description in status detail, got %q", out)
	}
}

func TestStatusCmd_RequirementWithAssignedStory(t *testing.T) {
	setupTestApp(t)
	rEvt := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-A", "title": "with agent", "description": "d", "repo_path": ".",
	})
	_ = app.projStore.Project(rEvt)
	sEvt := state.NewEvent(state.EventStoryCreated, "planner", "S-A", map[string]any{
		"id": "S-A", "req_id": "R-A", "title": "s", "description": "d",
		"acceptance_criteria": "a", "complexity": 1, "owned_files": []string{},
		"wave_hint": "parallel", "depends_on": []string{},
	})
	_ = app.projStore.Project(sEvt)
	aEvt := state.NewEvent(state.EventStoryAssigned, "AGENT1", "S-A", map[string]any{
		"agent_id": "AGENT1", "wave": 1,
	})
	_ = app.projStore.Project(aEvt)

	out := captureStdout(t, func() {
		cmd := newStatusCmd()
		cmd.SetArgs([]string{"R-A"})
		_ = cmd.Execute()
	})
	if !strings.Contains(out, "agent=AGENT1") {
		t.Errorf("expected agent= in story line, got %q", out)
	}
}

// --- gc ----------------------------------------------------------------------

func TestGCCmd_NoWorktreesDir(t *testing.T) {
	setupTestApp(t)
	out := captureStdout(t, func() {
		_ = newGCCmd().Execute()
	})
	if !strings.Contains(out, "0 worktrees") {
		t.Errorf("expected gc summary with 0 worktrees, got %q", out)
	}
}

func TestGCCmd_RemovesStale(t *testing.T) {
	dir := setupTestApp(t)
	// Create worktrees dir with one stale subdir.
	wDir := filepath.Join(dir, "worktrees")
	if err := os.MkdirAll(filepath.Join(wDir, "stale-req-id"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Also a file (should be skipped — not a dir).
	if err := os.WriteFile(filepath.Join(wDir, "stray.txt"), nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	out := captureStdout(t, func() {
		_ = newGCCmd().Execute()
	})
	if !strings.Contains(out, "Removed worktree") {
		t.Errorf("expected removal log, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(wDir, "stale-req-id")); !os.IsNotExist(err) {
		t.Errorf("stale dir should be removed, err=%v", err)
	}
}

func TestGCCmd_KeepsActive(t *testing.T) {
	dir := setupTestApp(t)
	// Seed an active requirement (planned status, not archived/completed).
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "ACTIVE", "title": "t", "description": "d", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}

	wDir := filepath.Join(dir, "worktrees")
	if err := os.MkdirAll(filepath.Join(wDir, "ACTIVE"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	out := captureStdout(t, func() {
		_ = newGCCmd().Execute()
	})
	if !strings.Contains(out, "0 worktrees") {
		t.Errorf("expected 0 worktrees removed when only active ones exist, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(wDir, "ACTIVE")); err != nil {
		t.Errorf("active worktree should not be removed: %v", err)
	}
}

// --- cost --------------------------------------------------------------------

func TestCostCmd_NoData(t *testing.T) {
	setupTestApp(t)
	out := captureStdout(t, func() {
		_ = newCostCmd().Execute()
	})
	if !strings.Contains(out, "Daily Cost") {
		t.Errorf("expected daily cost header, got %q", out)
	}
}

func TestCostCmd_WithRequirements(t *testing.T) {
	setupTestApp(t)
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-COST", "title": "cost test", "description": "d", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}
	out := captureStdout(t, func() {
		_ = newCostCmd().Execute()
	})
	if !strings.Contains(out, "R-COST") {
		t.Errorf("expected requirement id in cost output, got %q", out)
	}
}

func TestCostCmd_ClosedDB_ErrorsOnQuery(t *testing.T) {
	setupTestApp(t)
	// Close the projection store to force query errors in the cost ledger.
	if err := app.projStore.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	cmd := newCostCmd()
	if err := cmd.Execute(); err == nil {
		t.Error("expected cost cmd to error when DB is closed")
	}
}

func TestShowReqCost_ClosedDB(t *testing.T) {
	setupTestApp(t)
	// Seed first, then close.
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "RQ", "title": "t", "description": "d", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}
	if err := app.projStore.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	cmd := newCostCmd()
	cmd.SetArgs([]string{"RQ"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected showReqCost to error on closed db")
	}
}

func TestStatusCmd_ClosedDB(t *testing.T) {
	setupTestApp(t)
	if err := app.projStore.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := newStatusCmd().Execute(); err == nil {
		t.Error("expected status cmd to error on closed db")
	}
}

func TestStatusCmd_DetailMissing(t *testing.T) {
	setupTestApp(t)
	cmd := newStatusCmd()
	cmd.SetArgs([]string{"NOPE"})
	out := captureStdout(t, func() {
		_ = cmd.Execute()
	})
	_ = out
}

func TestArchiveCmd_AfterClose(t *testing.T) {
	setupTestApp(t)
	if err := app.projStore.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	cmd := newArchiveCmd()
	cmd.SetArgs([]string{"R"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected archive cmd to error on closed db")
	}
}

func TestAgentsCmd_AfterClose(t *testing.T) {
	setupTestApp(t)
	if err := app.projStore.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := newAgentsCmd().Execute(); err == nil {
		t.Error("expected agents cmd to error on closed db")
	}
}

func TestGCCmd_AfterClose(t *testing.T) {
	setupTestApp(t)
	if err := app.projStore.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := newGCCmd().Execute(); err == nil {
		t.Error("expected gc to error on closed db")
	}
}

func TestCostCmd_SpecificRequirement_NoStories(t *testing.T) {
	setupTestApp(t)
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-NS", "title": "no stories", "description": "d", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}
	out := captureStdout(t, func() {
		cmd := newCostCmd()
		cmd.SetArgs([]string{"R-NS"})
		_ = cmd.Execute()
	})
	if !strings.Contains(out, "No stories found") {
		t.Errorf("expected 'No stories found' for empty req cost, got %q", out)
	}
}

func TestCostCmd_SpecificRequirement(t *testing.T) {
	setupTestApp(t)
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-SP", "title": "specific", "description": "d", "repo_path": ".",
	})
	_ = app.projStore.Project(r)
	s := state.NewEvent(state.EventStoryCreated, "planner", "S-SP", map[string]any{
		"id": "S-SP", "req_id": "R-SP", "title": "s", "description": "d",
		"acceptance_criteria": "a", "complexity": 1, "owned_files": []string{},
		"wave_hint": "parallel", "depends_on": []string{},
	})
	_ = app.projStore.Project(s)

	out := captureStdout(t, func() {
		cmd := newCostCmd()
		cmd.SetArgs([]string{"R-SP"})
		_ = cmd.Execute()
	})
	if !strings.Contains(out, "Requirement:") || !strings.Contains(out, "R-SP") {
		t.Errorf("expected per-req cost output, got %q", out)
	}
	if !strings.Contains(out, "S-SP") {
		t.Errorf("expected story line in cost detail, got %q", out)
	}
}

// --- dashboard cmd ----------------------------------------------------------

func TestNewDashboardCmd_Flags(t *testing.T) {
	cmd := newDashboardCmd()
	for _, name := range []string{"web", "port", "bind"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
}

func TestNewDashboardCmd_WebFlagRoutesToWebServer(t *testing.T) {
	setupTestApp(t)

	cmd := newDashboardCmd()
	cmd.SetArgs([]string{"--web", "--port", "0", "--bind", "127.0.0.1"})

	// Cancel context immediately so the web server returns quickly.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd.SetContext(ctx)

	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("dashboard --web did not return after context cancel")
	}
}

// --- resume cmd flags -------------------------------------------------------

func TestNewResumeCmd_Flags(t *testing.T) {
	cmd := newResumeCmd()
	if cmd.Flags().Lookup("godmode") == nil {
		t.Error("missing --godmode flag")
	}
}

func TestNewResumeCmd_GodmodeFlag_PropagatesToRunResume(t *testing.T) {
	setupTestApp(t)
	cmd := newResumeCmd()
	cmd.SetArgs([]string{"--godmode", "MISSING"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for missing req under --godmode")
	}
}

func TestNewEventsCmd_LimitFlag(t *testing.T) {
	setupTestApp(t)
	for i := 0; i < 5; i++ {
		evt := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{"id": "X"})
		_ = app.eventStore.Append(evt)
	}
	cmd := newEventsCmd()
	cmd.SetArgs([]string{"--limit", "2"})
	out := captureStdout(t, func() { _ = cmd.Execute() })
	if strings.Count(out, "req.submitted") != 2 {
		t.Errorf("expected limit=2 to show 2 events, got %q", out)
	}
}

func TestRunResume_RequirementNotFound(t *testing.T) {
	setupTestApp(t)
	err := runResume(t.Context(), "missing", false)
	if err == nil {
		t.Error("expected error for missing requirement")
	}
}

func TestRunResume_ArchivedRequirement(t *testing.T) {
	setupTestApp(t)
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "ARCH", "title": "t", "description": "d", "repo_path": ".",
	})
	_ = app.projStore.Project(r)
	_ = app.projStore.ArchiveRequirement("ARCH")
	err := runResume(t.Context(), "ARCH", false)
	if err == nil || !strings.Contains(err.Error(), "archived") {
		t.Errorf("expected archived error, got %v", err)
	}
}

func TestRunResume_NoStories(t *testing.T) {
	setupTestApp(t)
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "EMPTY", "title": "t", "description": "d", "repo_path": ".",
	})
	_ = app.projStore.Project(r)
	err := runResume(t.Context(), "EMPTY", false)
	if err == nil || !strings.Contains(err.Error(), "no stories") {
		t.Errorf("expected 'no stories' error, got %v", err)
	}
}

func TestRunResume_NoStoriesReadyForDispatch(t *testing.T) {
	setupTestApp(t)

	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "DEPCYCLE", "title": "blocked", "description": "d", "repo_path": ".",
	})
	_ = app.projStore.Project(r)
	// Single story whose dependency points to a story that does NOT exist in
	// the requirement. The DAG will have an edge from an unknown id; dispatcher
	// won't be able to schedule the wave.
	s := state.NewEvent(state.EventStoryCreated, "planner", "S-BLOCK", map[string]any{
		"id": "S-BLOCK", "req_id": "DEPCYCLE", "title": "blocked", "description": "d",
		"acceptance_criteria": "a", "complexity": 1, "owned_files": []string{},
		"wave_hint": "parallel", "depends_on": []string{"S-NEVER"},
	})
	_ = app.projStore.Project(s)

	out := captureStdout(t, func() {
		_ = runResume(t.Context(), "DEPCYCLE", false)
	})
	if !strings.Contains(out, "No stories ready") && !strings.Contains(out, "Wave 1") {
		t.Logf("output: %q", out)
	}
}

func TestRunResume_SpawnFailsInNonGitRepo(t *testing.T) {
	setupTestApp(t)
	// Use a non-git directory as the repo path. executor.SpawnAll will fail to
	// create worktrees and runResume will hit the "No agents spawned" branch.
	nonGit := t.TempDir()

	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "NOGIT", "title": "no git", "description": "d", "repo_path": nonGit,
	})
	_ = app.projStore.Project(r)
	s := state.NewEvent(state.EventStoryCreated, "planner", "S-NOGIT", map[string]any{
		"id": "S-NOGIT", "req_id": "NOGIT", "title": "s", "description": "d",
		"acceptance_criteria": "a", "complexity": 1, "owned_files": []string{},
		"wave_hint": "parallel", "depends_on": []string{},
	})
	_ = app.projStore.Project(s)

	out := captureStdout(t, func() {
		if err := runResume(t.Context(), "NOGIT", false); err != nil {
			t.Logf("runResume returned %v (expected to fail-fast on spawn)", err)
		}
	})
	// We expect either an ERROR line per failed spawn or a "No agents spawned"
	// notice — both indicate the wave loop entered.
	if !strings.Contains(out, "Wave 1") {
		t.Errorf("expected wave dispatch attempt, got %q", out)
	}
}

func TestRunResume_AllStoriesAlreadyComplete(t *testing.T) {
	setupTestApp(t)

	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "DONE", "title": "all done", "description": "d", "repo_path": ".",
	})
	_ = app.projStore.Project(r)

	for _, sid := range []string{"S-1", "S-2"} {
		sEvt := state.NewEvent(state.EventStoryCreated, "planner", sid, map[string]any{
			"id": sid, "req_id": "DONE", "title": "s", "description": "d",
			"acceptance_criteria": "a", "complexity": 1, "owned_files": []string{},
			"wave_hint": "parallel", "depends_on": []string{},
		})
		_ = app.projStore.Project(sEvt)
		// Mark each story as merged.
		mEvt := state.NewEvent(state.EventStoryMerged, "monitor", sid, map[string]any{})
		_ = app.projStore.Project(mEvt)
	}

	out := captureStdout(t, func() {
		if err := runResume(t.Context(), "DONE", false); err != nil {
			t.Fatalf("runResume: %v", err)
		}
	})
	if !strings.Contains(out, "All stories are already complete") {
		t.Errorf("expected all-complete message, got %q", out)
	}
}
