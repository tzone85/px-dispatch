// Package planner decomposes natural-language requirements into atomic,
// independently implementable stories with dependencies, file ownership,
// and complexity scores. It uses a two-pass approach: Pass 1 generates
// stories via LLM, Validate checks quality, and if issues are found a
// second pass is attempted with validation feedback.
package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tzone85/px-dispatch/internal/llm"
)

// maxPlanningRounds is the maximum number of LLM calls the planner will
// make when validation issues are detected.
const maxPlanningRounds = 2

// PlannedStory represents a story produced by the planner.
type PlannedStory struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Complexity         int      `json:"complexity"`
	OwnedFiles         []string `json:"owned_files"`
	WaveHint           string   `json:"wave_hint"`
	DependsOn          []string `json:"depends_on"`
}

// PlannerConfig holds planner configuration.
type PlannerConfig struct {
	MaxStoryComplexity       int
	MaxStoriesPerRequirement int
	EnforceFileOwnership     bool
}

// Planner decomposes requirements into stories via LLM.
type Planner struct {
	client llm.Client
	config PlannerConfig
}

// NewPlanner creates a Planner with the given LLM client and configuration.
func NewPlanner(client llm.Client, cfg PlannerConfig) *Planner {
	return &Planner{client: client, config: cfg}
}

// Plan decomposes a requirement into stories using a two-pass approach.
// techStackInfo is optional context about the project's tech stack.
// If the first pass produces stories that fail validation, a second pass
// is attempted with the validation issues as feedback. If the second pass
// also fails validation, an error is returned.
func (p *Planner) Plan(ctx context.Context, requirement, techStackInfo string) ([]PlannedStory, error) {
	var lastIssues []string

	for round := 0; round < maxPlanningRounds; round++ {
		prompt := buildPlannerPrompt(requirement, techStackInfo, p.config, lastIssues)

		resp, err := p.client.Complete(ctx, llm.CompletionRequest{
			System:   plannerSystemPrompt,
			Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}},
		})
		if err != nil {
			return nil, fmt.Errorf("planner completion (round %d): %w", round+1, err)
		}

		stories, err := parseStories(resp.Content)
		if err != nil {
			lastIssues = []string{buildParseRetryIssue(resp.Content)}
			if round == maxPlanningRounds-1 {
				return nil, fmt.Errorf(
					"parse stories (round %d): %w; response excerpt: %q",
					round+1,
					err,
					summarizeModelOutput(resp.Content),
				)
			}
			continue
		}

		// If no validation config is set, skip validation and return immediately.
		if !hasValidationConfig(p.config) {
			return stories, nil
		}

		issues := Validate(stories, p.config)
		if len(issues) == 0 {
			return stories, nil
		}

		lastIssues = issues
	}

	return nil, fmt.Errorf("planner failed after %d rounds; unresolved issues: %s",
		maxPlanningRounds, strings.Join(lastIssues, "; "))
}

func buildParseRetryIssue(content string) string {
	return fmt.Sprintf(
		"Your previous response was not valid JSON. Return ONLY a valid JSON object that matches the required schema. Previous response excerpt: %q",
		summarizeModelOutput(content),
	)
}

func summarizeModelOutput(content string) string {
	summary := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	const maxSummaryLen = 200
	if len(summary) > maxSummaryLen {
		return summary[:maxSummaryLen] + "..."
	}
	return summary
}

// hasValidationConfig returns true if the config has any validation
// constraints set that would make Validate meaningful.
func hasValidationConfig(cfg PlannerConfig) bool {
	return cfg.MaxStoryComplexity > 0 ||
		cfg.MaxStoriesPerRequirement > 0 ||
		cfg.EnforceFileOwnership
}

// parseStories parses the LLM response content into stories.
// It handles real-world LLM output: markdown code fences, preamble text,
// both envelope format {"stories": [...]} and raw array [...].
func parseStories(content string) ([]PlannedStory, error) {
	cleaned := extractJSON(content)

	// Try envelope format: {"stories": [...]}
	var envelope struct {
		Stories []PlannedStory `json:"stories"`
	}
	if err := json.Unmarshal([]byte(cleaned), &envelope); err == nil && len(envelope.Stories) > 0 {
		return envelope.Stories, nil
	}

	// Try raw array: [...]
	var stories []PlannedStory
	if err := json.Unmarshal([]byte(cleaned), &stories); err == nil {
		return stories, nil
	}

	return nil, fmt.Errorf("unable to parse stories from LLM response: content is not valid JSON")
}

// extractJSON extracts JSON from LLM output that may contain markdown
// code fences, preamble text, or trailing explanation.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Strip markdown code fences: ```json ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		start := 1 // skip opening fence
		end := len(lines)
		if end > 0 && strings.TrimSpace(lines[end-1]) == "```" {
			end-- // skip closing fence
		}
		s = strings.Join(lines[start:end], "\n")
		s = strings.TrimSpace(s)
	}

	// If it starts with { or [, it's already JSON
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return s
	}

	// Find the first { or [ in the content (skip preamble text)
	braceIdx := strings.Index(s, "{")
	bracketIdx := strings.Index(s, "[")

	startIdx := -1
	if braceIdx >= 0 && (bracketIdx < 0 || braceIdx < bracketIdx) {
		startIdx = braceIdx
	} else if bracketIdx >= 0 {
		startIdx = bracketIdx
	}

	if startIdx < 0 {
		return s // no JSON found, return as-is (will fail parse)
	}

	// Find the matching closing brace/bracket from the end
	candidate := s[startIdx:]
	if strings.HasPrefix(candidate, "{") {
		lastBrace := strings.LastIndex(candidate, "}")
		if lastBrace >= 0 {
			return candidate[:lastBrace+1]
		}
	} else {
		lastBracket := strings.LastIndex(candidate, "]")
		if lastBracket >= 0 {
			return candidate[:lastBracket+1]
		}
	}

	return candidate
}

const plannerSystemPrompt = `You are a Tech Lead AI that decomposes software requirements into atomic, independently implementable stories.

RULES:
1. Each story MUST be atomic — one developer can implement it without touching other stories' files.
2. Every story MUST include ALL required fields:
   - id: unique identifier (e.g., "s-1", "s-2")
   - title: concise story title
   - description: what needs to be done
   - acceptance_criteria: how to verify completion
   - complexity: Fibonacci score (1, 2, 3, 5, 8, 13) reflecting implementation effort
   - owned_files: list of files this story will create or modify (non-overlapping with other stories)
   - wave_hint: "sequential" or "parallel" indicating if this can run alongside other stories
   - depends_on: list of story IDs that must complete before this story can start
3. File ownership MUST be explicit and non-overlapping — no two stories should own the same file.
4. Dependencies must form a DAG (no cycles).
5. Use Fibonacci complexity scoring: 1 (trivial), 2 (simple), 3 (moderate), 5 (complex), 8 (very complex), 13 (epic — consider breaking down further).

OUTPUT FORMAT:
Return ONLY valid JSON in this exact format:
{
  "stories": [
    {
      "id": "s-1",
      "title": "...",
      "description": "...",
      "acceptance_criteria": "...",
      "complexity": 3,
      "owned_files": ["file1.go", "file2.go"],
      "wave_hint": "sequential",
      "depends_on": []
    }
  ]
}

Do NOT include any text outside the JSON object.`

// buildPlannerPrompt constructs the user prompt for the planner LLM call.
func buildPlannerPrompt(requirement, techStackInfo string, cfg PlannerConfig, validationIssues []string) string {
	var b strings.Builder

	b.WriteString("## Requirement\n\n")
	b.WriteString(requirement)
	b.WriteString("\n")

	if techStackInfo != "" {
		b.WriteString("\n## Tech Stack\n\n")
		b.WriteString(techStackInfo)
		b.WriteString("\n")
	}

	if cfg.MaxStoryComplexity > 0 {
		b.WriteString(fmt.Sprintf("\n## Constraints\n\n- Maximum complexity per story: %d\n", cfg.MaxStoryComplexity))
	}
	if cfg.MaxStoriesPerRequirement > 0 {
		b.WriteString(fmt.Sprintf("- Maximum stories per requirement: %d\n", cfg.MaxStoriesPerRequirement))
	}
	if cfg.EnforceFileOwnership {
		b.WriteString("- File ownership must be non-overlapping (no two stories may own the same file)\n")
	}

	if len(validationIssues) > 0 {
		b.WriteString("\n## Previous Attempt Issues\n\n")
		b.WriteString("Your previous decomposition had the following issues. Please fix them:\n\n")
		for _, issue := range validationIssues {
			b.WriteString("- ")
			b.WriteString(issue)
			b.WriteString("\n")
		}
	}

	return b.String()
}
