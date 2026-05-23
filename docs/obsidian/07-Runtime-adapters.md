---
title: Runtime adapters
tags: [px-dispatch, runtime]
---

# Runtime adapters

A runtime is the AI CLI that an agent uses to do its work.

| Adapter | CLI | Default models | Cost tier |
|---|---|---|---|
| `claude-code` | `claude` | `claude-sonnet-4-…`, `claude-opus-4-…` | subscription |
| `codex` | `codex exec` | `gpt-5*`, `o3`, `o4-mini` | per-token |
| `gemini` | `gemini` | `gemini-2.5-pro`, `gemini-flash` | per-token |

Each implements `runtime.Runtime` with `Spawn`, `Kill`, `DetectStatus`,
`ReadOutput`, `SendInput`, and `Capabilities`.

## How a spawn actually works

All three runtimes invoke their CLI through `tmux new-session -d`:

```sh
rm -f .px-done
cat <<'PX_EOF' | claude … -p - | tee TRANSCRIPT.log
{rendered goal prompt}
PX_EOF
rc=$?
printf '$\n'
touch .px-done
sleep 30
exit $rc
```

`SupportsLogFile == true` does NOT mean "the CLI has a --output-file flag" —
the actual implementation shells out to `tee` for all three. We learned that
the hard way: the claude CLI's `--output-file` flag does not exist, and
passing it as a guess made claude exit immediately, killing the tmux pane
in <10 s.

## The router

`internal/runtime/router.go::SelectRuntime(role)` picks an adapter based on:

1. `routing.preferences[role].prefer` if set.
2. Cost-tier rule: subscription tier first if available.
3. Capability matching against the role's required model family.
4. Fallthrough: first registered runtime.

## Adding a new runtime

1. New file `internal/runtime/<name>.go` implementing the interface.
2. Adapt spawn script to the new CLI's invocation pattern. Keep the
   `rm -f .px-done` / `touch .px-done` markers — the poller depends on
   them.
3. Register in `runtime.NewRegistry()`.
4. Add `<name>_test.go` with `tee` / `rc=$?` regression assertions.
