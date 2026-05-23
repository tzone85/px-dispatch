// Package config provides typed configuration for px-dispatch with defaults,
// validation, and YAML loading.
package config

import "fmt"

// Config is the top-level configuration for px-dispatch.
type Config struct {
	Version   string                   `yaml:"version"`
	Workspace WorkspaceConfig          `yaml:"workspace"`
	Models    ModelsConfig             `yaml:"models"`
	Routing   RoutingConfig            `yaml:"routing"`
	Monitor   MonitorConfig            `yaml:"monitor"`
	Cleanup   CleanupConfig            `yaml:"cleanup"`
	Merge     MergeConfig              `yaml:"merge"`
	Planning  PlanningConfig           `yaml:"planning"`
	Budget    BudgetConfig             `yaml:"budget"`
	Sessions  SessionsConfig           `yaml:"sessions"`
	Pipeline  PipelineConfig           `yaml:"pipeline"`
	Fallback  FallbackConfig           `yaml:"fallback"`
	Runtimes  map[string]RuntimeConfig `yaml:"runtimes"`
	Pricing   map[string]PricingEntry  `yaml:"pricing"`
}

// WorkspaceConfig holds workspace-level settings.
type WorkspaceConfig struct {
	StateDir         string `yaml:"state_dir"`
	Backend          string `yaml:"backend"`
	LogLevel         string `yaml:"log_level"`
	LogRetentionDays int    `yaml:"log_retention_days"`
}

// ModelConfig represents a single LLM model reference.
type ModelConfig struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}

// ModelsConfig maps agent roles to their model configurations.
type ModelsConfig struct {
	TechLead     ModelConfig `yaml:"tech_lead"`
	Senior       ModelConfig `yaml:"senior"`
	Intermediate ModelConfig `yaml:"intermediate"`
	Junior       ModelConfig `yaml:"junior"`
	QA           ModelConfig `yaml:"qa"`
	Supervisor   ModelConfig `yaml:"supervisor"`
}

// RoutingConfig controls complexity-based task routing.
type RoutingConfig struct {
	JuniorMaxComplexity           int                 `yaml:"junior_max_complexity"`
	IntermediateMaxComplexity     int                 `yaml:"intermediate_max_complexity"`
	MaxRetriesBeforeEscalation    int                 `yaml:"max_retries_before_escalation"`
	MaxQAFailuresBeforeEscalation int                 `yaml:"max_qa_failures_before_escalation"`
	Strategy                      string              `yaml:"strategy"` // cost_optimized | performance
	Preferences                   []RoutingPreference `yaml:"preferences"`
}

// RoutingPreference maps an agent role to its preferred and fallback runtimes.
type RoutingPreference struct {
	Role     string `yaml:"role"`
	Prefer   string `yaml:"prefer"`
	Fallback string `yaml:"fallback"`
}

// MonitorConfig controls the supervisor monitor loop.
type MonitorConfig struct {
	PollIntervalMs         int `yaml:"poll_interval_ms"`
	StuckThresholdS        int `yaml:"stuck_threshold_s"`
	ContextFreshnessTokens int `yaml:"context_freshness_tokens"`
}

// CleanupConfig controls post-merge cleanup behaviour.
type CleanupConfig struct {
	WorktreePrune       string `yaml:"worktree_prune"`
	BranchRetentionDays int    `yaml:"branch_retention_days"`
	LogArchive          string `yaml:"log_archive"`
}

// MergeConfig controls merge and PR behaviour.
type MergeConfig struct {
	AutoMerge  bool   `yaml:"auto_merge"`
	BaseBranch string `yaml:"base_branch"`
	PRTemplate string `yaml:"pr_template"`
}

// PlanningConfig controls how requirements are decomposed into stories.
type PlanningConfig struct {
	SequentialFilePatterns   []string `yaml:"sequential_file_patterns"`
	MaxStoryComplexity       int      `yaml:"max_story_complexity"`
	MaxStoriesPerRequirement int      `yaml:"max_stories_per_requirement"`
	EnforceFileOwnership     bool     `yaml:"enforce_file_ownership"`
	Godmode                  bool     `yaml:"godmode"`
}

// BudgetConfig provides cost protection for LLM usage.
type BudgetConfig struct {
	MaxCostPerStoryUSD       float64 `yaml:"max_cost_per_story_usd"`
	MaxCostPerRequirementUSD float64 `yaml:"max_cost_per_requirement_usd"`
	MaxCostPerDayUSD         float64 `yaml:"max_cost_per_day_usd"`
	WarningThresholdPct      int     `yaml:"warning_threshold_pct"`
	HardStop                 bool    `yaml:"hard_stop"`
}

// SessionsConfig controls tmux session health management.
type SessionsConfig struct {
	StaleThresholdS     int    `yaml:"stale_threshold_s"`
	OnDead              string `yaml:"on_dead"`
	OnStale             string `yaml:"on_stale"`
	MaxRecoveryAttempts int    `yaml:"max_recovery_attempts"`
}

// PipelineConfig defines retry policies per pipeline stage.
type PipelineConfig struct {
	Stages map[string]StageConfig `yaml:"stages"`
}

// FallbackConfig controls graceful switching away from Claude when limits or
// billing exhaustion prevent progress.
type FallbackConfig struct {
	Enabled            bool   `yaml:"enabled"`
	RequireApproval    bool   `yaml:"require_approval"`
	LLMModel           string `yaml:"llm_model"`
	Runtime            string `yaml:"runtime"`
	RuntimeModel       string `yaml:"runtime_model"`
	HandoffOutputLines int    `yaml:"handoff_output_lines"`
}

// StageConfig is the retry policy for a single pipeline stage.
type StageConfig struct {
	MaxRetries int    `yaml:"max_retries"`
	OnExhaust  string `yaml:"on_exhaust"`
}

// RuntimeConfig describes an AI runtime (e.g., Claude Code, Cursor).
type RuntimeConfig struct {
	Command   string           `yaml:"command"`
	Args      []string         `yaml:"args"`
	Models    []string         `yaml:"models"`
	Detection RuntimeDetection `yaml:"detection"`
}

// RuntimeDetection holds regex patterns for detecting runtime states.
type RuntimeDetection struct {
	IdlePattern       string `yaml:"idle_pattern"`
	PermissionPattern string `yaml:"permission_pattern"`
	PlanModePattern   string `yaml:"plan_mode_pattern"`
}

// PricingEntry holds per-model token pricing.
type PricingEntry struct {
	InputPer1M  float64 `yaml:"input_per_1m"`
	OutputPer1M float64 `yaml:"output_per_1m"`
}

// Defaults returns a new Config populated with sensible default values.
// Each call returns an independent copy—callers may safely modify the result.
func Defaults() Config {
	return Config{
		Version: "1",
		Workspace: WorkspaceConfig{
			StateDir:         "~/.px",
			Backend:          "sqlite",
			LogLevel:         "info",
			LogRetentionDays: 30,
		},
		Budget: BudgetConfig{
			MaxCostPerStoryUSD:       2.0,
			MaxCostPerRequirementUSD: 20.0,
			MaxCostPerDayUSD:         50.0,
			WarningThresholdPct:      80,
			HardStop:                 true,
		},
		Routing: RoutingConfig{
			JuniorMaxComplexity:           3,
			IntermediateMaxComplexity:     5,
			MaxRetriesBeforeEscalation:    3,
			MaxQAFailuresBeforeEscalation: 2,
		},
		Monitor: MonitorConfig{
			PollIntervalMs:         10000,
			StuckThresholdS:        120,
			ContextFreshnessTokens: 150000,
		},
		Sessions: SessionsConfig{
			StaleThresholdS:     180,
			OnDead:              "redispatch",
			OnStale:             "restart",
			MaxRecoveryAttempts: 2,
		},
		Merge: MergeConfig{
			AutoMerge:  true,
			BaseBranch: "main",
		},
		Cleanup: CleanupConfig{
			WorktreePrune:       "immediate",
			BranchRetentionDays: 7,
		},
		Planning: PlanningConfig{
			MaxStoryComplexity:       8,
			MaxStoriesPerRequirement: 15,
			EnforceFileOwnership:     true,
		},
		Fallback: FallbackConfig{
			Enabled:            false,
			RequireApproval:    true,
			LLMModel:           "gpt-5.4",
			Runtime:            "codex",
			RuntimeModel:       "gpt-5.4",
			HandoffOutputLines: 80,
		},
	}
}

// Validate checks that all configuration values are within acceptable bounds.
func (c Config) Validate() error {
	if err := c.validateWorkspace(); err != nil {
		return err
	}
	if err := c.validateRouting(); err != nil {
		return err
	}
	if err := c.validateRuntimeModelAlignment(); err != nil {
		return err
	}
	if err := c.validateFallback(); err != nil {
		return err
	}
	if err := c.validateBudget(); err != nil {
		return err
	}
	if err := c.validateSessions(); err != nil {
		return err
	}
	if err := c.validateCleanup(); err != nil {
		return err
	}
	return nil
}

func (c Config) validateFallback() error {
	if !c.Fallback.Enabled {
		return nil
	}

	if c.Fallback.LLMModel == "" {
		return fmt.Errorf("fallback.llm_model must be set when fallback.enabled=true")
	}
	if c.Fallback.Runtime == "" {
		return fmt.Errorf("fallback.runtime must be set when fallback.enabled=true")
	}
	if c.Fallback.RuntimeModel == "" {
		return fmt.Errorf("fallback.runtime_model must be set when fallback.enabled=true")
	}
	if c.Fallback.HandoffOutputLines < 10 {
		return fmt.Errorf("fallback.handoff_output_lines must be >= 10, got %d", c.Fallback.HandoffOutputLines)
	}

	return nil
}

func (c Config) validateWorkspace() error {
	validBackends := map[string]bool{"sqlite": true, "dolt": true}
	if !validBackends[c.Workspace.Backend] {
		return fmt.Errorf("workspace.backend must be 'sqlite' or 'dolt', got %q", c.Workspace.Backend)
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Workspace.LogLevel] {
		return fmt.Errorf("workspace.log_level must be debug/info/warn/error, got %q", c.Workspace.LogLevel)
	}

	return nil
}

func (c Config) validateRouting() error {
	if c.Routing.JuniorMaxComplexity < 1 || c.Routing.JuniorMaxComplexity > 13 {
		return fmt.Errorf("routing.junior_max_complexity must be 1-13, got %d", c.Routing.JuniorMaxComplexity)
	}

	if c.Routing.IntermediateMaxComplexity < c.Routing.JuniorMaxComplexity {
		return fmt.Errorf(
			"routing.intermediate_max_complexity (%d) must be >= junior_max_complexity (%d)",
			c.Routing.IntermediateMaxComplexity,
			c.Routing.JuniorMaxComplexity,
		)
	}

	if c.Routing.IntermediateMaxComplexity > 13 {
		return fmt.Errorf("routing.intermediate_max_complexity must be <= 13, got %d", c.Routing.IntermediateMaxComplexity)
	}

	return nil
}

func (c Config) validateRuntimeModelAlignment() error {
	for _, pref := range c.Routing.Preferences {
		modelCfg := c.modelConfigForRole(pref.Role)
		if modelCfg.Provider == "" {
			continue
		}

		if err := validateRuntimeProvider(pref.Role, "prefer", pref.Prefer, modelCfg.Provider); err != nil {
			return err
		}
		if err := validateRuntimeProvider(pref.Role, "fallback", pref.Fallback, modelCfg.Provider); err != nil {
			return err
		}
	}

	return nil
}

func (c Config) modelConfigForRole(role string) ModelConfig {
	switch role {
	case "tech_lead":
		return c.Models.TechLead
	case "senior":
		return c.Models.Senior
	case "intermediate":
		return c.Models.Intermediate
	case "junior":
		return c.Models.Junior
	case "qa":
		return c.Models.QA
	case "supervisor":
		return c.Models.Supervisor
	default:
		return ModelConfig{}
	}
}

func validateRuntimeProvider(role, field, runtimeName, provider string) error {
	if runtimeName == "" {
		return nil
	}

	expectedProvider, ok := runtimeProviders[runtimeName]
	if !ok || provider == "" {
		return nil
	}

	if normalizeProvider(provider) != expectedProvider {
		return fmt.Errorf(
			"routing.preferences %q runtime %q requires models.%s.provider to be %q, got %q",
			field, runtimeName, role, expectedProvider, provider,
		)
	}

	return nil
}

func normalizeProvider(provider string) string {
	switch provider {
	case "anthropic", "Anthropic":
		return "anthropic"
	case "openai", "OpenAI":
		return "openai"
	case "google", "Google", "gemini", "Gemini":
		return "google"
	default:
		return provider
	}
}

var runtimeProviders = map[string]string{
	"claude-code": "anthropic",
	"codex":       "openai",
	"gemini":      "google",
}

func (c Config) validateBudget() error {
	if c.Budget.MaxCostPerStoryUSD < 0 {
		return fmt.Errorf("budget.max_cost_per_story_usd must be non-negative, got %f", c.Budget.MaxCostPerStoryUSD)
	}
	if c.Budget.MaxCostPerRequirementUSD < 0 {
		return fmt.Errorf("budget.max_cost_per_requirement_usd must be non-negative, got %f", c.Budget.MaxCostPerRequirementUSD)
	}
	if c.Budget.MaxCostPerDayUSD < 0 {
		return fmt.Errorf("budget.max_cost_per_day_usd must be non-negative, got %f", c.Budget.MaxCostPerDayUSD)
	}
	if c.Budget.WarningThresholdPct < 0 {
		return fmt.Errorf("budget.warning_threshold_pct must be non-negative, got %d", c.Budget.WarningThresholdPct)
	}

	return nil
}

func (c Config) validateSessions() error {
	validOnDead := map[string]bool{"redispatch": true, "escalate": true, "pause": true}
	if !validOnDead[c.Sessions.OnDead] {
		return fmt.Errorf("sessions.on_dead must be redispatch/escalate/pause, got %q", c.Sessions.OnDead)
	}

	validOnStale := map[string]bool{"restart": true, "kill_redispatch": true, "escalate": true}
	if !validOnStale[c.Sessions.OnStale] {
		return fmt.Errorf("sessions.on_stale must be restart/kill_redispatch/escalate, got %q", c.Sessions.OnStale)
	}

	return nil
}

func (c Config) validateCleanup() error {
	validPrune := map[string]bool{"immediate": true, "deferred": true}
	if !validPrune[c.Cleanup.WorktreePrune] {
		return fmt.Errorf("cleanup.worktree_prune must be 'immediate' or 'deferred', got %q", c.Cleanup.WorktreePrune)
	}

	return nil
}
