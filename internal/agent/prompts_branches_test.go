package agent

import (
	"strings"
	"testing"
)

// Exercises every conditional branch in SystemPrompt and GoalPrompt added
// when we ported the VXD prompts (IsExistingCodebase, IsBugFix,
// IsInfrastructure, WaveContext, ReviewFeedback, DesignApproach).
// Each test asserts the diagnostic playbook OR design-approach block
// appears in the rendered prompt — not just that the prompt compiled.

func TestSystemPrompt_ExistingCodebase_InjectsArchaeologyForTechLead(t *testing.T) {
	out := SystemPrompt(RoleTechLead, PromptContext{StoryID: "s", IsExistingCodebase: true})
	if !strings.Contains(out, "Codebase Archaeology") {
		t.Errorf("expected CodebaseArchaeology in tech-lead prompt, got %q", excerpt(out))
	}
}

func TestSystemPrompt_ExistingCodebase_InjectsBugHuntForSenior(t *testing.T) {
	out := SystemPrompt(RoleSenior, PromptContext{StoryID: "s", IsExistingCodebase: true})
	if !strings.Contains(out, "Bug Hunting") {
		t.Errorf("expected BugHuntingMethodology in senior prompt, got %q", excerpt(out))
	}
	if !strings.Contains(out, "Legacy") {
		t.Errorf("expected LegacyCodeSurvival in senior prompt, got %q", excerpt(out))
	}
}

func TestSystemPrompt_ExistingCodebase_LegacyCodeForJuniorIntermediate(t *testing.T) {
	for _, r := range []Role{RoleJunior, RoleIntermediate} {
		out := SystemPrompt(r, PromptContext{StoryID: "s", IsExistingCodebase: true})
		if !strings.Contains(out, "Legacy") {
			t.Errorf("%s prompt should include LegacyCodeSurvival, got %q", r, excerpt(out))
		}
	}
}

func TestSystemPrompt_BugFix_InjectsForSeniorWhenNotExisting(t *testing.T) {
	out := SystemPrompt(RoleSenior, PromptContext{StoryID: "s", IsBugFix: true})
	if !strings.Contains(out, "Bug Hunting") {
		t.Errorf("expected BugHuntingMethodology, got %q", excerpt(out))
	}
}

func TestSystemPrompt_BugFix_DedupedWithExisting(t *testing.T) {
	// When both IsExistingCodebase AND IsBugFix are set, we should NOT
	// double-inject BugHuntingMethodology.
	out := SystemPrompt(RoleSenior, PromptContext{
		StoryID: "s", IsExistingCodebase: true, IsBugFix: true,
	})
	occurrences := strings.Count(out, "Bug Hunting Methodology")
	if occurrences > 1 {
		t.Errorf("BugHuntingMethodology should appear at most once, found %d times", occurrences)
	}
}

func TestSystemPrompt_Infrastructure_InjectsForAnyRole(t *testing.T) {
	for _, r := range []Role{RoleTechLead, RoleSenior, RoleIntermediate, RoleJunior} {
		out := SystemPrompt(r, PromptContext{StoryID: "s", IsInfrastructure: true})
		if !strings.Contains(out, "Infrastructure") {
			t.Errorf("role %s: expected InfrastructureDebugging block, got %q", r, excerpt(out))
		}
	}
}

func TestGoalPrompt_ExistingCodebase_MandatoryWorkflow(t *testing.T) {
	out := GoalPrompt(RoleIntermediate, PromptContext{
		StoryID: "s", StoryTitle: "x", IsExistingCodebase: true,
	})
	if !strings.Contains(out, "EXISTING CODEBASE — MANDATORY WORKFLOW") {
		t.Errorf("expected existing-codebase workflow block, got %q", excerpt(out))
	}
}

func TestGoalPrompt_BugFix_MandatoryWorkflow(t *testing.T) {
	out := GoalPrompt(RoleSenior, PromptContext{
		StoryID: "s", StoryTitle: "fix nil panic", IsBugFix: true,
	})
	if !strings.Contains(out, "BUG FIX — MANDATORY WORKFLOW") {
		t.Errorf("expected bug-fix workflow, got %q", excerpt(out))
	}
}

func TestGoalPrompt_Infrastructure_DiagnosticSequence(t *testing.T) {
	out := GoalPrompt(RoleSenior, PromptContext{
		StoryID: "s", StoryTitle: "docker compose", IsInfrastructure: true,
	})
	if !strings.Contains(out, "INFRASTRUCTURE — DIAGNOSTIC SEQUENCE") {
		t.Errorf("expected infrastructure diagnostic block, got %q", excerpt(out))
	}
}

func TestGoalPrompt_WaveContext_IncludesPriorWork(t *testing.T) {
	out := GoalPrompt(RoleJunior, PromptContext{
		StoryID: "s", StoryTitle: "x",
		WaveContext: "Story s-1 created the User entity.",
	})
	if !strings.Contains(out, "What Prior Stories Built") {
		t.Errorf("expected wave-context header, got %q", excerpt(out))
	}
	if !strings.Contains(out, "User entity") {
		t.Errorf("expected wave context body, got %q", excerpt(out))
	}
}

func TestGoalPrompt_DDD_TDD_BlockDefault(t *testing.T) {
	out := GoalPrompt(RoleIntermediate, PromptContext{StoryID: "s", StoryTitle: "x"})
	if !strings.Contains(out, "MANDATORY: Domain-Driven Design + Test-Driven Development") {
		t.Errorf("expected DDD+TDD block by default, got %q", excerpt(out))
	}
}

func TestGoalPrompt_TDDOnly_WhenConfigured(t *testing.T) {
	out := GoalPrompt(RoleJunior, PromptContext{
		StoryID: "s", StoryTitle: "x", DesignApproach: "tdd",
	})
	if !strings.Contains(out, "MANDATORY: Test-Driven Development") {
		t.Errorf("expected TDD-only block, got %q", excerpt(out))
	}
	if strings.Contains(out, "Domain-Driven Design") {
		t.Errorf("TDD-only should not include DDD block, got %q", excerpt(out))
	}
}

func TestGoalPrompt_Standard_NoExtraDesignBlock(t *testing.T) {
	out := GoalPrompt(RoleJunior, PromptContext{
		StoryID: "s", StoryTitle: "x", DesignApproach: "standard",
	})
	if strings.Contains(out, "Test-Driven Development") {
		t.Errorf("standard approach should not include TDD block")
	}
	if strings.Contains(out, "Domain-Driven Design") {
		t.Errorf("standard approach should not include DDD block")
	}
}

func TestGoalPrompt_ReviewFeedback_FailureRecovery(t *testing.T) {
	out := GoalPrompt(RoleJunior, PromptContext{
		StoryID: "s", StoryTitle: "x",
		ReviewFeedback: "tests are missing for handler.go",
	})
	if !strings.Contains(out, "Previous Review Feedback") {
		t.Errorf("expected feedback header, got %q", excerpt(out))
	}
	if !strings.Contains(out, "tests are missing for handler.go") {
		t.Errorf("expected feedback body, got %q", excerpt(out))
	}
}

func TestGoalPrompt_ComplexityIncludedInPrompt(t *testing.T) {
	out := GoalPrompt(RoleSenior, PromptContext{
		StoryID: "s", StoryTitle: "x", Complexity: 8,
	})
	if !strings.Contains(out, "Complexity: 8") {
		t.Errorf("expected complexity line, got %q", excerpt(out))
	}
}

func excerpt(s string) string {
	if len(s) <= 200 {
		return s
	}
	return s[:200] + "…"
}
