---
title: Agent prompts and DDD+TDD enforcement
tags: [project-x, prompts, ddd, tdd]
---

# Agent prompts and DDD+TDD enforcement

Agent prompts are where Project X enforces engineering standards. They live
in `internal/agent/prompts.go`; the diagnostic playbooks they inject live in
`internal/agent/diagnostics.go`. Both were ported from `vortex-dispatch`
(see `SHARED_LEARNINGS.md` for lineage).

## How a prompt is built

```
base role template (tech_lead | senior | intermediate | junior | qa | supervisor)
  + diagnostic playbook injection
    - CodebaseArchaeology       if ctx.IsExistingCodebase, RoleTechLead
    - BugHuntingMethodology     if ctx.IsBugFix
    - LegacyCodeSurvival        if ctx.IsExistingCodebase
    - InfrastructureDebugging   if ctx.IsInfrastructure
  + WaveContext (from WAVE_CONTEXT.md if present)
  + DDD+TDD block (default DesignApproach)
  + ReviewFeedback (when a previous attempt was rejected)
```

The classifiers `detectExistingCodebase`, `classifyIsBugFix`, and
`classifyIsInfrastructure` live in `internal/monitor/prompt_classifiers.go`
and run automatically — the caller doesn't have to label the story.

## What DDD+TDD enforcement means in practice

When `DesignApproach == "ddd-tdd"` (the default), every goal prompt ships
with:

- **TDD workflow** — Red → Green → Refactor. Write a failing test FIRST.
- **DDD patterns** — entities, value objects, repositories, services, use
  cases, DTOs. Business logic stays out of frameworks.
- **File organisation** — separate domain from infrastructure, group by
  feature.
- **Dependency injection** — pass repositories/services as parameters.
- **Boundary validation** — validate at the controller/route, not deep in
  domain logic.

The [[04-Pipeline-stages-walkthrough|review stage]] checks the diff against
the spec's `OwnedFiles`. Behaviour that "works" but lands in the wrong file
(CSS inlined into HTML, business logic in a route handler) is rejected.

## The 5W1H pre-plan

The tech-lead template forces a six-dimension analysis BEFORE writing
stories: WHAT, WHO, WHEN, WHERE, WHY, HOW. For each unanswered dimension,
the tech-lead must make an explicit decision. Key decisions are documented
in the first story so all downstream agents share context.

## Why this exists

The VXD tic-tac-toe pilot (SHARED_LEARNINGS.md, finding #5) caught a case
where an agent put all CSS into `<style>` blocks inside `index.html` even
though the spec named `styles.css` as the owned file — and the LLM reviewer
rubber-stamped it because the game worked. The fix is on both ends: the
agent has explicit DDD+TDD guidance with file ownership, and the reviewer
fails stories on structural mismatch, not just functional defects.
