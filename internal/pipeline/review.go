package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tzone85/project-x/internal/git"
	"github.com/tzone85/project-x/internal/llm"
)

// reviewResponse is the expected JSON structure from the LLM review.
type reviewResponse struct {
	Passed   bool     `json:"passed"`
	Summary  string   `json:"summary"`
	Comments []string `json:"comments"`
}

// ReviewStage sends the story diff to an LLM for code review. The LLM
// returns a structured verdict indicating whether the changes pass review.
//
// The reviewer is told to fail the story when:
//   - the deliverable structure (files actually changed) does NOT match the
//     spec's stated OwnedFiles list,
//   - the implementation places business logic inline when the spec calls for
//     a dedicated file (e.g. styles inline in HTML when CSS file is owned),
//   - there are unhandled errors, hardcoded secrets, or obvious security holes.
//
// These constraints come from VXD's tic-tac-toe pilot (SHARED_LEARNINGS.md
// findings #1, #5): the LLM previously rubber-stamped stories where the
// functional behaviour matched but the structural spec did not.
type ReviewStage struct {
	runner    git.CommandRunner
	llmClient llm.Client
}

// NewReviewStage creates a ReviewStage with the given runner and LLM client.
func NewReviewStage(runner git.CommandRunner, client llm.Client) *ReviewStage {
	return &ReviewStage{runner: runner, llmClient: client}
}

// Name returns the stage identifier.
func (s *ReviewStage) Name() string { return "review" }

// Execute gets the diff and file tree, sends them to the LLM for review,
// and parses the structured response.
func (s *ReviewStage) Execute(ctx context.Context, sc StoryContext) (StageResult, error) {
	diff, err := s.getDiff(sc)
	if err != nil {
		return StageFailed, fmt.Errorf("getting diff for review: %w", err)
	}

	fileTree, err := s.runner.Run(sc.WorktreePath, "git", "ls-files")
	if err != nil {
		return StageFailed, fmt.Errorf("listing files: %w", err)
	}

	prompt := buildReviewPrompt(sc.StoryID, sc.StoryTitle, sc.StoryDescription, sc.AcceptanceCriteria, sc.OwnedFiles, diff, fileTree)
	resp, err := s.llmClient.Complete(ctx, llm.CompletionRequest{
		System: reviewerSystemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		if llm.IsFatalAPIError(err) {
			return StageFatal, fmt.Errorf("LLM review: %w", err)
		}
		return StageFailed, fmt.Errorf("LLM review: %w", err)
	}

	var review reviewResponse
	if err := json.Unmarshal([]byte(resp.Content), &review); err != nil {
		return StageFailed, fmt.Errorf("parsing review response: %w", err)
	}

	if !review.Passed {
		return StageFailed, fmt.Errorf("review failed: %s", review.Summary)
	}

	return StagePassed, nil
}

// getDiff retrieves the full diff against the base branch.
func (s *ReviewStage) getDiff(sc StoryContext) (string, error) {
	baseBranch := sc.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	mergeBase, err := s.runner.Run(sc.WorktreePath, "git", "merge-base", "HEAD", "origin/"+baseBranch)
	if err != nil {
		return "", fmt.Errorf("finding merge-base: %w", err)
	}

	diff, err := s.runner.Run(sc.WorktreePath, "git", "diff", mergeBase)
	if err != nil {
		return "", fmt.Errorf("running diff: %w", err)
	}

	return diff, nil
}

// reviewerSystemPrompt is the system prompt for the code-review LLM. It must
// fail stories on BOTH functional defects AND spec/structure mismatches.
const reviewerSystemPrompt = `You are a strict senior code reviewer for an autonomous AI dev team.

You receive a story spec (title, description, acceptance criteria, owned files)
and the diff produced by an agent. Decide PASS or FAIL.

Fail the story when ANY of the following is true:
1. STRUCTURAL MISMATCH — the spec listed files the agent must own/produce, and
   the diff did not touch them, OR put work that belonged in those files
   somewhere else (e.g. CSS inlined into HTML when styles.css is the owned
   file). Functional behaviour matching is NOT enough.
2. UNFULFILLED ACCEPTANCE CRITERIA — any item in the criteria is not
   demonstrably satisfied by the diff.
3. NO TESTS or tests assert implementation details rather than behaviour.
4. UNHANDLED ERRORS / SILENT FAILURES — discarded error returns, empty catch
   blocks, swallowed promise rejections, ` + "`" + `panic` + "`" + ` instead of returning err.
5. SECURITY ISSUE — hardcoded secrets, SQL or shell injection vector, missing
   input validation at a system boundary, unsanitised HTML rendering.
6. BROKEN INVARIANT — diff removes or weakens an existing test, type, or
   public contract without justification in the description.

Respond ONLY with strict JSON:
{"passed": bool, "summary": "one-line verdict", "comments": ["specific issue 1", "specific issue 2"]}

Comments must include file:line references where applicable.`

// buildReviewPrompt constructs the LLM prompt for code review.
func buildReviewPrompt(storyID, title, description, acceptance string, ownedFiles []string, diff, fileTree string) string {
	owned := "(none specified)"
	if len(ownedFiles) > 0 {
		owned = ""
		for _, f := range ownedFiles {
			owned += "- " + f + "\n"
		}
	}
	return fmt.Sprintf(
		"Review the following changes for story %s.\n\n"+
			"## Title\n%s\n\n"+
			"## Description\n%s\n\n"+
			"## Acceptance Criteria\n%s\n\n"+
			"## Spec-Declared Owned Files\n%s\n"+
			"## File Tree (current state)\n```\n%s\n```\n\n"+
			"## Diff (changes to review)\n```diff\n%s\n```\n\n"+
			"Apply the spec-vs-output check FIRST, then functional review. "+
			"Respond with JSON: {\"passed\": bool, \"summary\": string, \"comments\": [string]}",
		storyID, title, description, acceptance, owned, fileTree, diff,
	)
}
