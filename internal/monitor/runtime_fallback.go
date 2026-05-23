package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tzone85/px-dispatch/internal/agent"
	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/modelswitch"
	"github.com/tzone85/px-dispatch/internal/runtime"
	"github.com/tzone85/px-dispatch/internal/state"
)

// RuntimeFallbackManager handles Claude-to-OpenAI runtime handoff when the
// current agent cannot continue because Claude account limits were reached.
type RuntimeFallbackManager struct {
	runner     git.CommandRunner
	registry   *runtime.Registry
	cfg        config.FallbackConfig
	eventStore state.EventStore
	projector  EventSender
	approver   modelswitch.Approver
}

type handoffSnapshot struct {
	Reason                 string   `json:"reason"`
	StoryID                string   `json:"story_id"`
	StoryTitle             string   `json:"story_title"`
	Role                   string   `json:"role"`
	WorktreePath           string   `json:"worktree_path"`
	CurrentRuntime         string   `json:"current_runtime"`
	CurrentModel           string   `json:"current_model"`
	CurrentBranch          string   `json:"current_branch"`
	TranscriptPath         string   `json:"transcript_path,omitempty"`
	TranscriptSnapshot     string   `json:"transcript_snapshot,omitempty"`
	TranscriptSnapshotFile string   `json:"transcript_snapshot_file,omitempty"`
	RecentPaneOutput       string   `json:"recent_pane_output,omitempty"`
	GitStatus              string   `json:"git_status,omitempty"`
	DiffStat               string   `json:"diff_stat,omitempty"`
	ChangedFiles           []string `json:"changed_files,omitempty"`
}

// NewRuntimeFallbackManager creates a runtime handoff manager.
func NewRuntimeFallbackManager(
	runner git.CommandRunner,
	registry *runtime.Registry,
	cfg config.FallbackConfig,
	eventStore state.EventStore,
	projector EventSender,
	approver modelswitch.Approver,
) *RuntimeFallbackManager {
	if !cfg.Enabled {
		return nil
	}
	return &RuntimeFallbackManager{
		runner:     runner,
		registry:   registry,
		cfg:        cfg,
		eventStore: eventStore,
		projector:  projector,
		approver:   approver,
	}
}

// TrySwitch replaces a completed Claude session with the configured fallback
// runtime when the pane output shows Claude account exhaustion.
func (m *RuntimeFallbackManager) TrySwitch(ctx context.Context, ag ActiveAgent, output string) (ActiveAgent, bool, error) {
	if ag.RuntimeName != "claude-code" || !isAgentDone(output) {
		return ag, false, nil
	}

	reason, ok := modelswitch.DetectClaudeExhaustion(output)
	if !ok {
		return ag, false, nil
	}

	if m.cfg.RequireApproval && m.approver != nil {
		approved, err := m.approver.ApproveSwitch(modelswitch.Request{
			Scope:           modelswitch.ScopeRuntime,
			Operation:       "story execution",
			StoryID:         ag.Assignment.StoryID,
			StoryTitle:      ag.Story.Title,
			CurrentProvider: "anthropic",
			CurrentRuntime:  ag.RuntimeName,
			TargetProvider:  "openai",
			TargetRuntime:   m.cfg.Runtime,
			TargetModel:     m.cfg.RuntimeModel,
			Reason:          reason,
			Note:            "The replacement agent will continue in the same worktree and branch with a generated handoff summary.",
		})
		if err != nil {
			m.pauseRequirement(ag, fmt.Sprintf("Claude fallback approval failed: %v", err))
			return ag, false, err
		}
		if !approved {
			err := fmt.Errorf("model switch declined for story %s", ag.Assignment.StoryID)
			m.pauseRequirement(ag, err.Error())
			return ag, false, err
		}
	}

	nextRuntime, err := m.registry.Get(m.cfg.Runtime)
	if err != nil {
		m.pauseRequirement(ag, fmt.Sprintf("fallback runtime %q is not registered", m.cfg.Runtime))
		return ag, false, err
	}

	snapshot := m.captureHandoffSnapshot(ag, reason, output)
	if err := m.writeHandoffArtifacts(ag.WorktreePath, snapshot); err != nil {
		m.emitProgress(ag, "runtime_fallback", "handoff_write_failed", map[string]any{"error": err.Error()})
	}

	promptCtx := agent.PromptContext{
		StoryID:            ag.Story.ID,
		StoryTitle:         ag.Story.Title,
		StoryDescription:   ag.Story.Description,
		AcceptanceCriteria: ag.Story.AcceptanceCriteria,
		RepoPath:           ag.WorktreePath,
		Complexity:         ag.Story.Complexity,
	}

	sessionCfg := runtime.SessionConfig{
		SessionName: ag.Assignment.SessionName,
		WorkDir:     ag.WorktreePath,
		Model:       m.cfg.RuntimeModel,
		SystemPrompt: agent.SystemPrompt(ag.Assignment.Role, promptCtx) +
			"\n\nYou are continuing work started by a previous Claude agent. Preserve existing changes and continue from the current branch state.",
		Goal: agent.GoalPrompt(ag.Assignment.Role, promptCtx) +
			"\n\n### Continuation Instructions\n" +
			"Claude could not continue because: " + reason + "\n" +
			"Do not start over. First read " + handoffFileName + " and " + handoffJSONFileName +
			", inspect the current git diff, and continue from the existing branch state.",
	}

	if err := nextRuntime.Spawn(m.runner, sessionCfg); err != nil {
		m.pauseRequirement(ag, fmt.Sprintf("failed to spawn fallback runtime %s/%s: %v", m.cfg.Runtime, m.cfg.RuntimeModel, err))
		return ag, false, err
	}

	m.emitProgress(ag, "runtime_fallback", "switched", map[string]any{
		"from_runtime": ag.RuntimeName,
		"to_runtime":   m.cfg.Runtime,
		"to_model":     m.cfg.RuntimeModel,
		"reason":       reason,
	})

	updated := ag
	updated.RuntimeName = nextRuntime.Name()
	return updated, true, nil
}

func (m *RuntimeFallbackManager) pauseRequirement(ag ActiveAgent, reason string) {
	m.emitProgress(ag, "runtime_fallback", "paused", map[string]any{
		"reason": reason,
	})
	if ag.Assignment.ReqID == "" {
		return
	}
	m.emit(state.NewEvent(state.EventReqPaused, "fallback", ag.Assignment.StoryID, map[string]any{
		"req_id": ag.Assignment.ReqID,
	}))
}

func (m *RuntimeFallbackManager) captureHandoffSnapshot(ag ActiveAgent, reason, output string) handoffSnapshot {
	transcriptPath := ag.TranscriptPath
	if transcriptPath == "" {
		transcriptPath = filepath.Join(ag.WorktreePath, transcriptFileName)
	}

	snapshot := handoffSnapshot{
		Reason:           reason,
		StoryID:          ag.Assignment.StoryID,
		StoryTitle:       ag.Story.Title,
		Role:             string(ag.Assignment.Role),
		WorktreePath:     ag.WorktreePath,
		CurrentRuntime:   ag.RuntimeName,
		CurrentModel:     ag.Model,
		CurrentBranch:    m.bestEffortRun(ag.WorktreePath, "git", "rev-parse", "--abbrev-ref", "HEAD"),
		TranscriptPath:   transcriptPath,
		RecentPaneOutput: tailLines(output, m.cfg.HandoffOutputLines),
		GitStatus:        m.bestEffortRun(ag.WorktreePath, "git", "status", "--short"),
		DiffStat:         m.bestEffortRun(ag.WorktreePath, "git", "diff", "--stat"),
		ChangedFiles:     nonEmptyLines(m.bestEffortRun(ag.WorktreePath, "git", "diff", "--name-only")),
	}

	if transcriptSnapshot := tailLines(readFileIfExists(transcriptPath), m.cfg.HandoffOutputLines); transcriptSnapshot != "" {
		snapshot.TranscriptSnapshot = transcriptSnapshot
		snapshot.TranscriptSnapshotFile = transcriptSnapshotFileName
	}

	return snapshot
}

func (m *RuntimeFallbackManager) writeHandoffArtifacts(worktreePath string, snapshot handoffSnapshot) error {
	if snapshot.TranscriptSnapshot != "" {
		if err := os.WriteFile(filepath.Join(worktreePath, transcriptSnapshotFileName), []byte(snapshot.TranscriptSnapshot), 0o644); err != nil {
			return fmt.Errorf("write transcript snapshot: %w", err)
		}
	}

	if err := os.WriteFile(filepath.Join(worktreePath, handoffFileName), []byte(buildHandoffSummary(snapshot)), 0o644); err != nil {
		return fmt.Errorf("write markdown handoff: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal handoff json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, handoffJSONFileName), data, 0o644); err != nil {
		return fmt.Errorf("write json handoff: %w", err)
	}

	return nil
}

func buildHandoffSummary(snapshot handoffSnapshot) string {
	var b strings.Builder

	b.WriteString("# Runtime Handoff\n\n")
	b.WriteString("Claude could not continue this story.\n\n")
	b.WriteString("## Reason\n\n")
	b.WriteString(snapshot.Reason)
	b.WriteString("\n\n")
	b.WriteString("## Story\n\n")
	b.WriteString(snapshot.StoryTitle)
	b.WriteString("\n\n")

	if snapshot.CurrentBranch != "" {
		b.WriteString("## Branch\n\n```text\n")
		b.WriteString(snapshot.CurrentBranch)
		b.WriteString("\n```\n\n")
	}

	if snapshot.GitStatus != "" {
		b.WriteString("## Git Status\n\n```text\n")
		b.WriteString(snapshot.GitStatus)
		b.WriteString("\n```\n\n")
	}

	if snapshot.DiffStat != "" {
		b.WriteString("## Diff Stat\n\n```text\n")
		b.WriteString(snapshot.DiffStat)
		b.WriteString("\n```\n\n")
	}

	if len(snapshot.ChangedFiles) > 0 {
		b.WriteString("## Changed Files\n\n")
		for _, file := range snapshot.ChangedFiles {
			b.WriteString("- ")
			b.WriteString(file)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if snapshot.TranscriptSnapshot != "" {
		b.WriteString("## Transcript Snapshot\n\n```text\n")
		b.WriteString(snapshot.TranscriptSnapshot)
		b.WriteString("\n```\n\n")
	}

	if snapshot.RecentPaneOutput != "" {
		b.WriteString("## Recent Claude Output\n\n```text\n")
		b.WriteString(snapshot.RecentPaneOutput)
		b.WriteString("\n```\n")
	}

	return b.String()
}

func (m *RuntimeFallbackManager) bestEffortRun(dir, name string, args ...string) string {
	out, err := m.runner.Run(dir, name, args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func readFileIfExists(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func nonEmptyLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func (m *RuntimeFallbackManager) emitProgress(ag ActiveAgent, stage, status string, extra map[string]any) {
	payload := map[string]any{
		"stage":  stage,
		"status": status,
	}
	for k, v := range extra {
		payload[k] = v
	}
	m.emit(state.NewEvent(state.EventStoryProgress, "fallback", ag.Assignment.StoryID, payload))
}

func (m *RuntimeFallbackManager) emit(evt state.Event) {
	_ = m.eventStore.Append(evt)
	if m.projector != nil {
		m.projector.Send(evt)
	}
}

func tailLines(s string, n int) string {
	if n <= 0 {
		return strings.TrimSpace(s)
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return strings.TrimSpace(strings.Join(lines[len(lines)-n:], "\n"))
}
