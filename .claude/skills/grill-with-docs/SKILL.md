---
name: grill-with-docs
description: Interrogate a plan against the project's documented decisions and domain language — challenging fuzzy terms, cross-referencing the code, and surfacing where the plan contradicts an existing ADR or canonical contract. Use when the user wants a plan stress-tested against what the project has already decided, rather than in the abstract. Prefer plain grill-me when there is no documented model to test against, and plan-context-loader when the goal is to gather vault context rather than be questioned.
allowed-tools: Read, Grep, Glob, Bash, Agent, mcp__obsidian__view, mcp__obsidian__get_workspace_files
---

<what-to-do>

Interview me relentlessly about every aspect of this plan until we reach a shared understanding. Walk down each branch of the design tree, resolving dependencies between decisions one-by-one. For each question, provide your recommended answer.

Ask the questions one at a time, waiting for feedback on each question before continuing.

If a question can be answered by exploring the codebase, explore the codebase instead.

</what-to-do>

<supporting-info>

## Domain awareness

In this repo the documented model lives in the Obsidian vault under `docs/`, not in a root
`CONTEXT.md`. Read from these before challenging anything:

| Authority | Where |
|---|---|
| Accepted decisions | `docs/ADR/` (6-digit files, `status: accepted`, no inbound `supersedes`) |
| Canonical contracts | `docs/plan/` — e.g. `knowledge-trail-core-concept.md` |
| Architecture invariants | `docs/wiki/architecture/`, `docs/wiki/HOME.md` |
| Known gaps / remediation | `docs/review/` |
| Operational constraints | `docs/runbooks/` |

`python3 scripts/adr_graph.py resolve <id>` gives the current successor of an ADR, so you challenge
against the live decision rather than a superseded one.

There is no glossary file in this repo today. When a term genuinely needs pinning down, resolve it in
the conversation and record it in the relevant `docs/plan/` contract — and ask before creating any new
document, since an unrequested glossary file becomes a second source of truth nobody maintains.

## During the session

### Challenge against the documented language

When the user uses a term that conflicts with how the vault defines it, call it out immediately.
"`knowledge-trail-core-concept.md` uses 'footprint' for X, but you seem to mean Y — which is it?"

Watch for vocabulary from superseded designs being used as if current — Knowledge Loop terms
(4-bucket, primary surface `/loop`) are historical per [[000940]] and must not be treated as the
live contract.

### Sharpen fuzzy language

When the user uses vague or overloaded terms, propose a precise canonical term. "You're saying 'account' — do you mean the Customer or the User? Those are different things."

### Discuss concrete scenarios

When domain relationships are being discussed, stress-test them with specific scenarios. Invent scenarios that probe edge cases and force the user to be precise about the boundaries between concepts.

### Cross-reference with code

When the user states how something works, check whether the code agrees. If you find a contradiction, surface it: "Your code cancels entire Orders, but you just said partial cancellation is possible — which is right?"

### Capture resolved terms as you go

When a term gets pinned down, note it in your running summary rather than batching it to the end —
the precision is what the session is for, and it evaporates if you defer it.

Writing it into a `docs/plan/` contract is a separate, user-approved edit. Ask first.

### Offer ADRs sparingly

Only offer to create an ADR when all three are true:

1. **Hard to reverse** — the cost of changing your mind later is meaningful
2. **Surprising without context** — a future reader will wonder "why did they do it this way?"
3. **The result of a real trade-off** — there were genuine alternatives and you picked one for specific reasons

If any of the three is missing, skip the ADR.

When one is warranted, hand off to the **alt-adr-writer** skill rather than inventing a format here.
This repo has one ADR convention — 6-digit numbering, `docs/ADR/template.md` sections, wikilink
`[[000NNN]]` cross-references, a fixed tag list — and a second format competing with it produces
records that Obsidian's graph and `scripts/adr_graph.py` cannot read.

</supporting-info>
