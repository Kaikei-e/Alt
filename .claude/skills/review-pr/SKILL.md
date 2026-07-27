---
name: review-pr
description: Review a GitHub pull request against the failure modes specific to Alt — Clean Architecture layer direction, missing CDC coverage for a crossed service boundary, unwired components that unit tests still pass, and immutable-data-model violations. Use when the user asks to review a PR by number or URL. The bundled /review skill covers the generic code-quality pass; this one adds what Alt has been burned by.
allowed-tools: Bash, Read, Grep, Glob, Agent
argument-hint: <pr-number>
---

# Review a Pull Request

Target PR: `$ARGUMENTS`

## Gather

```bash
gh pr view $ARGUMENTS
gh pr diff $ARGUMENTS
gh pr checks $ARGUMENTS
```

## Alt-specific dimensions

Each of these has shipped a production incident, which is why they are worth a dedicated pass rather
than being left to a general read of the diff.

1. **Layer direction** — a Driver importing Usecase, a Usecase importing infrastructure, or a Handler
   doing Driver work. Delegate a broad sweep to the `layer-checker` agent; use the
   `clean-architecture` skill for the rules.
2. **Crossed service boundary without CDC** — a new or changed required header, field, auth
   requirement, or proto message needs every consumer's pact updated *and* verified by the provider.
   See `tdd-workflow` Phase 1b.
3. **Unwired components** — a constructor that exists, has passing unit tests, and is never
   referenced from `main`/`di`. Grep the composition root and look for the `*_enabled` startup log
   (`.claude/rules/di-wiring.md`).
4. **Silent fallback** — `if x == nil { return nil }` in a producer, projector or resolver path makes
   "DI forgot to wire it" indistinguishable from "intentionally disabled".
5. **Immutable-model violations** — new `UPDATE`s on an append-only table, a projector reading latest
   state, business-fact `time.Now()`. Use the `immutable-design-guard` skill when the diff touches
   migrations, projectors or event handlers.
6. **Security** — use the `security-auditor` skill in `--mode=diff` rather than restating OWASP here.
7. **Test coverage** — new behavior arrived with a test that would fail without the change.

## Report

Group findings by dimension, most severe first. For each: file and line, what breaks, and the
concrete fix. Note the CI status, and say explicitly which dimensions you checked and found clean —
an unmentioned dimension reads as unchecked.
