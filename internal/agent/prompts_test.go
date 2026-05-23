package agent

import (
	"strings"
	"testing"
)

func TestSystemPrompt_ContainsRole(t *testing.T) {
	ctx := PromptContext{
		StoryID:    "s-1",
		StoryTitle: "Add auth",
	}
	prompt := SystemPrompt(RoleSenior, ctx)
	if !strings.Contains(strings.ToLower(prompt), "senior") {
		t.Error("system prompt should mention the role")
	}
}

func TestSystemPrompt_TechLead_DifferentFromJunior(t *testing.T) {
	ctx := PromptContext{StoryID: "s-1"}
	tlPrompt := SystemPrompt(RoleTechLead, ctx)
	jrPrompt := SystemPrompt(RoleJunior, ctx)
	if tlPrompt == jrPrompt {
		t.Error("tech lead and junior should have different system prompts")
	}
}

func TestSystemPrompt_AllRolesProduceNonEmpty(t *testing.T) {
	ctx := PromptContext{StoryID: "s-1", StoryTitle: "Test story"}
	for _, role := range AllRoles() {
		prompt := SystemPrompt(role, ctx)
		if len(prompt) == 0 {
			t.Errorf("system prompt for role %s should not be empty", role)
		}
	}
}

func TestGoalPrompt_ContainsStoryDetails(t *testing.T) {
	ctx := PromptContext{
		StoryID:            "s-1",
		StoryTitle:         "Add auth",
		StoryDescription:   "Implement OAuth2 login",
		AcceptanceCriteria: "Users can log in via Google",
		Complexity:         5,
	}
	prompt := GoalPrompt(RoleIntermediate, ctx)
	if !strings.Contains(prompt, "Add auth") {
		t.Error("goal should contain story title")
	}
	if !strings.Contains(prompt, "OAuth2") {
		t.Error("goal should contain description")
	}
	if !strings.Contains(prompt, "Users can log in") {
		t.Error("goal should contain acceptance criteria")
	}
}

func TestGoalPrompt_IncludesReviewFeedback(t *testing.T) {
	ctx := PromptContext{
		StoryID:        "s-1",
		StoryTitle:     "Add auth",
		ReviewFeedback: "Missing error handling in login endpoint",
	}
	prompt := GoalPrompt(RoleJunior, ctx)
	if !strings.Contains(prompt, "Missing error handling") {
		t.Error("goal should include review feedback when present")
	}
}

func TestGoalPrompt_NoFeedbackSection_WhenEmpty(t *testing.T) {
	ctx := PromptContext{
		StoryID:    "s-1",
		StoryTitle: "Add auth",
	}
	prompt := GoalPrompt(RoleJunior, ctx)
	if strings.Contains(prompt, "Review Feedback") {
		t.Error("should not include feedback section when empty")
	}
}

func TestGoalPrompt_ContainsComplexity(t *testing.T) {
	ctx := PromptContext{
		StoryID:    "s-1",
		StoryTitle: "Add auth",
		Complexity: 8,
	}
	prompt := GoalPrompt(RoleSenior, ctx)
	if !strings.Contains(prompt, "8") {
		t.Error("goal should contain complexity score")
	}
}

func TestGoalPrompt_ContainsStoryID(t *testing.T) {
	ctx := PromptContext{
		StoryID:    "s-42",
		StoryTitle: "Add auth",
	}
	prompt := GoalPrompt(RoleSenior, ctx)
	if !strings.Contains(prompt, "s-42") {
		t.Error("goal should contain story ID")
	}
}
