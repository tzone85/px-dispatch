---
title: Lessons from pilots
tags: [project-x, pilots, learnings]
---

# Lessons from pilots

Every pilot surfaces findings the unit tests missed. Append new findings
here as they emerge.

## VXD tic-tac-toe (2026-05-22, from SHARED_LEARNINGS.md)

5 issues, all ported to Project X as fixes BEFORE our own pilots ran:

1. **YAML int-key gotcha.** Map keys must be bare ints (`5: 60`), not
   strings (`"5": 60`).
2. **Provider name aliasing.** `anthropic_cli` should alias `claude-cli`.
3. **`WAVE_CONTEXT.md` leaks into user's working tree.** Pipeline cleanup
   must strip OR commit it.
4. **Merger leaves story stuck mid-rebase.** Pipeline didn't detect a
   stale `rebase-merge` dir. → Fixed by
   [[05-Conflict-resolution-and-rebase-guard|stale-rebase guard]].
5. **Reviewer rubber-stamps spec-noncompliant work.** Agent inlined CSS
   into HTML when `styles.css` was owned; review still passed.
   → Fixed by spec-compliance gate in
   [[04-Pipeline-stages-walkthrough|review stage]].

## Project X E2E verification (2026-05-22)

Three independent bugs surfaced via the `/version` endpoint pilot:

1. **`--output-file` is not a real claude flag.** → `tee` redirect.
2. **Poller treated session-end as `agent.lost`.** → `.px-done` filesystem
   sentinel + poller check.
3. **`status=$?` is read-only in zsh.** → `rc=$?`.

## Connect-4 greenfield (2026-05-22, in progress)

Findings being logged:

- **QA stage too strict on scaffolding stories.** Story 1 was "Scaffold
  Vite React+TS project"; agent wrote no test files (correct — TDD comes
  next story); vitest exits with code 1 + "No test files found". Pipeline
  treated this as failure and respawned. → Fixed by `isNoTestsError` in
  `internal/pipeline/qa.go`: vitest/jest/pytest/go-test "no tests yet"
  messages no longer fail the stage. The review stage's spec-compliance
  check still catches stories where tests WERE expected.

## SpeedReading revamp (queued)

Target: `tzone85/SpeedReading` (Expo + Fastify monorepo, ~6.8k LOC). Will
exercise:
- `IsExistingCodebase` → injects `CodebaseArchaeology` into tech-lead
  prompt + `LegacyCodeSurvival` into implementers.
- Whether tech-lead actually runs the archaeology before planning.
- Whether review flags violations of existing patterns even when spec
  doesn't mention them.

## How to add findings

When a pilot surfaces a real issue:

1. Section with date + project name.
2. State the issue in one sentence.
3. Link the fix commit / PR.
4. Update [[03-Agent-prompts-and-DDD-TDD-enforcement]] or
   [[04-Pipeline-stages-walkthrough]] if the fix is a prompt or stage
   change.

Cross-link to `SHARED_LEARNINGS.md` at the misc root for the longer-form
record across sibling projects.
