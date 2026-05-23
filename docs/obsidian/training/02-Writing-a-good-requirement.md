---
title: Lesson 02 · Writing a good requirement
tags: [px-dispatch, training, requirements]
---

# Lesson 02 · Writing a good requirement

**Goal:** Be able to predict, when you write a requirement, what the tech-lead
will decompose it into. ~20 minutes; no LLM spend (analysis only).

The biggest predictor of pilot success is requirement quality. The tech-lead
prompt is a strong LLM but it can only decompose what's there. This lesson is
a side-by-side of bad vs good requirements with concrete fixes.

## The five tests

A requirement passes if:

1. **Goal in one sentence.** Vague goals get vague stories.
2. **Functional behaviour as bullets, testable.** Each bullet is the seed of
   an acceptance-criterion check.
3. **Code quality bar.** DDD layering, TDD, lint settings, tsconfig strict,
   coverage threshold.
4. **Explicit owned files.** The [[../04-Pipeline-stages-walkthrough#Stage 3 — review (LLM judge)|reviewer's structural check]] uses this
   list. Skip it and the agent silently puts everything in one component.
5. **Concrete acceptance criteria.** `npm run build` passes, `npm test`
   passes, etc. Not "looks good."

## Case study: a bad requirement

```
Make me a CRUD app for managing books.
```

Tech-lead will produce 4-7 stories, but each will be sloppy: schema in the
controller, no tests, types named `any`. The reviewer can't enforce file
structure because no `OwnedFiles` were specified.

## Same requirement, rewritten

```markdown
# Bookshelf CRUD (Fastify + TypeScript + Prisma + Postgres)

## Goal
A REST API that lets a single user create, list, update, and delete books.
No auth, no UI — API only.

## Functional behaviour
- POST   /books          → create  → 201 + body { id, title, author }
- GET    /books          → list    → 200 + [ ... ]
- GET    /books/:id      → read    → 200 + body | 404
- PATCH  /books/:id      → update  → 200 + body | 404 | 400 on validation
- DELETE /books/:id      → delete  → 204 | 404

## Code quality
- DDD: domain layer (Book entity, repository interface) separate from
  Fastify routes. Application service mediates.
- TDD: every domain function has a vitest test written first.
- TypeScript strict mode, no `any`, ESLint zero warnings.
- Prisma for persistence; Postgres via docker-compose for local dev.
- Validation at the route boundary (Zod), not in the domain.

## Owned files (exact)
- src/domain/book.ts                 — Book entity + value objects
- src/domain/book.test.ts            — domain unit tests
- src/domain/bookRepository.ts       — repository interface
- src/application/bookService.ts     — use cases (createBook, etc.)
- src/application/bookService.test.ts
- src/infrastructure/prismaBookRepository.ts
- src/routes/books.ts                — Fastify route handlers (thin)
- src/routes/books.integration.test.ts
- src/app.ts                         — Fastify composition
- prisma/schema.prisma
- docker-compose.yml                 — Postgres service for tests
- package.json, tsconfig.json, vitest.config.ts

## Acceptance criteria
- `npm run build` succeeds.
- `npm test` passes; domain + application coverage ≥ 95%.
- `npm run lint` passes with zero warnings.
- `docker compose up -d postgres && npm test` (integration tests) passes.
- POST → GET → PATCH → DELETE roundtrip works against a fresh DB.
```

That same intent now produces ~9 stories in a deterministic DDD order
(domain → repository → service → infrastructure → routes → integration),
each with a precise `OwnedFiles` list the reviewer can enforce.

## Exercises

### Exercise A — diagnose

For each of the requirements below, list which of the five tests it fails:

1. `Add OAuth login.`
2. `Build a real-time chat with WebSockets. Tests must pass. DDD please.`
3. `Add a /version endpoint with comprehensive coverage and clean code.`

(Answers at the bottom.)

### Exercise B — rewrite

Take Exercise A #2 and turn it into a passing requirement of your own. Use
the case-study template above as a skeleton. Pay extra attention to the
**owned files** list — chat has stateful and stateless components and the
agent needs to know which file gets the connection lifecycle.

### Exercise C — predict

Run `px plan <your-requirement.txt>`. **Before** running `px plan --review`,
write down on paper how many stories you expect and in what DDD order
(domain → application → infrastructure → routes). Then run review and
compare. The closer your prediction, the better the requirement.

## Answers to Exercise A

1. Fails 1, 2, 3, 4, 5 — pure verbal intent, no specifics.
2. Fails 1, 4, 5 — DDD is mentioned but undefined; no file list; "tests must
   pass" is not concrete.
3. Fails 1 (vague "version"), 4 (no file list), 5 ("comprehensive coverage"
   is not a number).

## Anti-patterns to avoid

- "Whatever you think is best" → defers all decisions to the implementing
  agent, who hasn't read the codebase.
- "Match the existing style" without naming the style → the agent guesses.
- Listing 30 files in `OwnedFiles` → the reviewer's structural check trips
  on subtle drift. Keep to 5-15 per requirement, split into more
  requirements if you need more.
- Putting acceptance criteria as prose paragraphs → the LLM struggles to
  match them. Bullets every time.

Next: [[03-Reading-the-event-log]] — once you can write requirements, learn
to read what px is doing with them.
