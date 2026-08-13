---
name: grill-with-docs
description: >-
  Interrogates a plan or design the user already has — one question at a time, walking every
  branch of the decision tree — and checks each answer against Alt's recorded decisions, namely
  the ADRs in `docs/ADR/`, the canonical contracts in `docs/plan/`, and the code itself.
  Surfaces contradictions with an accepted ADR, vocabulary borrowed from superseded designs, and
  terms too fuzzy to build on. Use whenever the user wants a plan stress-tested, grilled, or
  thought through out loud — "grill me", "poke holes in this design", "stress-test this plan",
  「このプランを詰めて」「設計を叩いて」「ADR と矛盾してないか見て」 — even if they never mention
  the docs. Prefer plan-context-loader when the user has no plan yet and just needs the vault
  context gathered before designing.
allowed-tools: Read, Grep, Glob, Bash, Agent
---

<what-to-do>

Interview the user relentlessly about this plan until you reach a shared understanding. Walk down each
branch of the design tree, resolving dependencies between decisions one by one. Ask **one question at a
time** and wait for the answer before continuing. For each question, give your own recommended answer.

If a question can be answered by exploring the codebase or the vault, explore instead of asking.

</what-to-do>

<supporting-info>

## Where the documented model lives

Alt has no root `CONTEXT.md` and no glossary file — the documented model is the Obsidian vault under
`docs/`. Read from these before challenging anything:

| Authority | Where |
|---|---|
| Accepted decisions | `docs/ADR/` (6-digit files, `status: accepted`, no inbound `supersedes`) |
| Canonical contracts | `docs/plan/` — e.g. `knowledge-trail-core-concept.md` |
| Architecture invariants | `docs/wiki/architecture/`, `docs/wiki/HOME.md` |
| Known gaps / remediation | `docs/review/` |
| Operational constraints | `docs/runbooks/` |

`docdag resolve <id>` returns the current successor of an ADR, so you challenge the live decision
rather than a superseded one.

## During the session

**Challenge against the documented language.** When a term conflicts with how the vault defines it, say
so immediately: "`knowledge-trail-core-concept.md` uses 'footprint' for X, but you seem to mean Y —
which is it?" Watch especially for Knowledge Loop vocabulary (4-bucket, primary surface `/loop`) being
used as if current — it is historical per [[000940]].

**Sharpen fuzzy terms.** Propose a precise canonical term when one is overloaded: "You're saying
'account' — do you mean the Customer or the User? Those are different things."

**Stress-test with concrete scenarios.** Invent cases that probe the edges and force precision about
where one concept ends and the next begins.

**Cross-reference with code.** When the user states how something works, check whether the code agrees,
and surface the contradiction: "Your code cancels entire Orders, but you just said partial cancellation
is possible — which is right?"

**Capture resolved terms as you go**, in a running summary rather than batching them to the end — the
precision is what the session is for, and it evaporates if you defer it. Writing a term into a
`docs/plan/` contract is a separate, user-approved edit: ask first. An unrequested glossary file becomes
a second source of truth nobody maintains.

## Offer an ADR sparingly

Only offer one when all three hold:

1. **Hard to reverse** — changing your mind later has a real cost
2. **Surprising without context** — a future reader will ask "why did they do it this way?"
3. **The result of a real trade-off** — there were genuine alternatives and one was picked for reasons

If any is missing, skip the ADR. When one is warranted, hand off to the **alt-adr-writer** skill instead
of inventing a format here — Alt has one ADR convention (6-digit numbering, `docs/ADR/template.md`
sections, `[[000NNN]]` wikilinks, a fixed tag list), and a competing format produces records that
Obsidian's graph and `docdag` cannot read.

</supporting-info>
