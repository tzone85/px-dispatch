---
title: Project X · Systematic Walkthrough
tags: [project-x, index]
---

# Project X · Systematic Walkthrough

**Repo:** [github.com/tzone85/project-x](https://github.com/tzone85/project-x)
**Local path:** `/Users/mncedimini/Sites/misc/project-x`

This vault is the top-to-bottom map of the system. Read in order, or jump.

## Reading order

1. [[01-What-Project-X-is]]
2. [[02-Architecture-at-a-glance]]
3. [[03-Agent-prompts-and-DDD-TDD-enforcement]]
4. [[04-Pipeline-stages-walkthrough]]
5. [[05-Conflict-resolution-and-rebase-guard]]
6. [[06-Cost-protection-and-budget-breakers]]
7. [[07-Runtime-adapters]]
8. [[08-Web-dashboard-and-API]]
9. [[09-Operating-the-system]]
10. [[10-Lessons-from-pilots]]
11. [[11-Open-questions]]

## One-sentence mental model

`px` reads a requirement, asks an LLM tech-lead to decompose it into atomic
DDD-shaped stories, dispatches AI coding agents into isolated git worktrees,
and drives every story through a seven-stage pipeline until merged — with
cost budgets, health watchdogs, and a fire-and-forget cleanup at the end.

## Cross-links

- For the canonical spec: `docs/superpowers/specs/2026-05-22-architecture-reference.md`
- For onboarding: `docs/superpowers/specs/2026-05-22-onboarding.md`
