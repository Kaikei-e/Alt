---
title: Knowledge Loop Projector Event Matrix
date: 2026-04-27
status: accepted
tags:
  - knowledge-loop
  - projector
  - reproject-safe
  - canonical-contract
aliases:
  - knowledge-loop-projector-matrix
---

# Knowledge Loop Projector Event Matrix

This document is the canonical mapping between knowledge_event types and the
projector's read-model writes. It is bound to the source code by a
parser test (`TestProjectorMatrix_DocCovered` in
`knowledge-sovereign/app/usecase/knowledge_loop_projector/projector_matrix_doc_test.go`):
the test parses the table below and cross-checks each event-type literal
against the constants block in `constants.go`. Drift in either direction
fails CI.

Reading guidance:

- **Entry**: does the projector upsert / patch a `knowledge_loop_entries` row?
- **Session**: does the projector update `knowledge_loop_session_state`?
- **Surface**: does the projector recompute `knowledge_loop_surfaces`?
- **Bucket**: which surface bucket the entry lands in (see canonical contract §6).
- **Why**: which `WhyKind` the projector tags on the entry's `why_primary`.
- **Act target hint**: what default `act_targets[]` the enricher seeds (callers
  may override by overlaying a more specific target downstream).

The matrix below is **the** source of truth for which side-effects each event
produces. Adding a new event type requires (a) appending it to `constants.go`,
(b) adding a row here, and (c) wiring the projector. The parser test catches
omissions in either step.

## Matrix

| Event type | Entry | Session | Surface | Bucket | WhyKind | Act target hint |
|---|---|---|---|---|---|---|
| ArticleCreated | yes | no | no | — | — | — |
| SummaryVersionCreated | yes | no | yes | now | source_why | article, ask |
| SummaryNarrativeBackfilled | patch | no | no | — | source_why | (preserved) |
| HomeItemsSeen | no | no | no | — | — | — |
| HomeItemOpened | yes | yes | yes | continue | recall_why | article, ask |
| HomeItemDismissed | patch | no | yes | review | recall_why | (preserved) |
| HomeItemAsked | yes | yes | yes | continue | recall_why | article, ask |
| SummarySuperseded | yes | no | yes | changed | change_why | diff, ask |
| HomeItemSuperseded | yes | no | yes | changed | change_why | diff, ask |
| knowledge_loop.observed.v1 | yes | yes | yes | now | source_why | article, ask |
| knowledge_loop.oriented.v1 | yes | yes | yes | continue | recall_why | article, ask |
| knowledge_loop.decision_presented.v1 | yes | yes | yes | continue | pattern_why | article, ask |
| knowledge_loop.acted.v1 | yes | yes | yes | continue | recall_why | article, ask |
| knowledge_loop.returned.v1 | yes | yes | yes | continue | recall_why | article, ask |
| knowledge_loop.deferred.v1 | patch | yes | yes | review | recall_why | (preserved) |
| knowledge_loop.reviewed.v1 | patch | yes | yes | review | recall_why | (preserved) |
| knowledge_loop.session_reset.v1 | no | yes | no | — | — | — |
| knowledge_loop.lens_mode_switched.v1 | no | yes | no | — | — | — |
| recap.topic_snapshotted.v1 | no | no | no | — | — | (Surface Planner v2 input only) |
| augur.conversation_linked.v1 | no | no | no | — | — | (Surface Planner v2 input only) |
| knowledge_loop.surface_plan_recomputed.v1 | no | no | no | — | — | (system-only; never user-emittable) |

## Canonical invariants reflected by this matrix

1. **Single emission** (canonical contract §3 invariant 7) — the `HomeItem*`
   rows write to Knowledge Loop projection because the same physical user
   action produces exactly one event from exactly one UI lane. `/loop` UI
   emits `knowledge_loop.*.v1`; `/feeds` UI emits `HomeItem*`. Both lanes
   share the projector, but no single user action results in both kinds of
   event being appended.
2. **Reproject-safe** (§3 invariant 1) — every "yes" in the Surface column
   must be derivable from event payload + versioned artifacts. The projector
   never reads "latest" cross-table state to produce these effects.
3. **Patch vs upsert** — `patch` rows update only the columns named by the
   event (e.g. `dismiss_state` for `HomeItemDismissed`,
   `why_text`/`why_evidence_refs` for `SummaryNarrativeBackfilled`) and
   preserve every other column verbatim. `upsert` rows take full ownership.
4. **System-only events** — `knowledge_loop.surface_plan_recomputed.v1` is
   emitted by the projector itself (or, more precisely, by the scheduler
   wrapping the projector — see Stream 2B in the active plan). It MUST NOT
   appear on the user-emittable trigger allowlist; the BFF rejects it on the
   user transition path.
