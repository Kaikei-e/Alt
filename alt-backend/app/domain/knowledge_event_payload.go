package domain

// Knowledge event payload structs.
//
// These types are the single source of truth for the wire schema of every
// payload that the alt-backend marshals into knowledge_events.payload. All
// producers (outbox_worker, connect/v2/internal article-created handler,
// knowledge_backfill_job) MUST marshal through these structs — using
// ad-hoc map[string]any literals re-introduces the wire-form drift class
// of bug PM-2026-041 / ADR-000865 documented.
//
// Terminology used inside Alt for the article source URL:
//
//   - "URL"  — the canonical concept. The HTTP(S) location of the article
//     itself (e.g. https://example.com/blog/post). Stored authoritatively in
//     the producer-side `articles.url` column (alt-backend DB) and exposed
//     to the world through this `URL` field with the canonical wire key
//     "url". All new code must use this name.
//
//   - "Link" — the legacy projection-era naming. The same value as URL,
//     stored in `knowledge_home_items.link` column and `KnowledgeHomeItem.Link`
//     Go field. The DB column and the Go field name are NOT renamed here
//     — that is a wider-blast-radius migration covered by a separate ADR.
//     What is renamed here is exclusively the JSON wire key inside event
//     payloads: legacy `"link"` → canonical `"url"`.
//
//   - "wire key" — the JSON key name on the bytes inside
//     knowledge_events.payload. Canonical: `"url"`. Legacy (forbidden for
//     new events): `"link"`. Historical events with the legacy key are
//     repaired forward via `ArticleUrlBackfilledPayload` corrective events
//     (append-first; no event mutation, no consumer dual-key fallback).

// ArticleCreatedPayload is the canonical wire schema for the
// `ArticleCreated` knowledge event. The `URL` field is marshalled as
// `"url"` — the canonical key — and producers must populate it from the
// article's source URL (`articles.url`). Empty URL is permitted by this
// type alone (no validation here) — URL scheme allowlisting and emptiness
// rejection live one level up at each producer call site so the marshaller
// stays a pure data carrier.
type ArticleCreatedPayload struct {
	ArticleID   string `json:"article_id"`
	Title       string `json:"title"`
	PublishedAt string `json:"published_at"`
	TenantID    string `json:"tenant_id"`
	URL         string `json:"url"`
}

// ArticleUrlBackfilledPayload is the corrective event payload for repairing
// historical `ArticleCreated` events whose payload was written with the
// legacy `"link"` key (or no URL key at all). The projector applies this
// event as a partial patch on the existing `knowledge_home_items.link`
// column (only that column is updated — title, score, why_reasons, etc.
// are preserved). Reproject-safe: depends only on this payload's bytes.
//
// This is intentionally a separate event type rather than a re-emission
// of `ArticleCreated`: the dedupe registry would block a second
// `ArticleCreated` for the same article_id, and even if we used a
// different dedupe namespace, replay order would mean the projector
// re-runs the original (link-empty) `ArticleCreated` after the corrective
// one. A distinct event type lets the projector apply a patch-only
// branch that survives any replay order via the seq-hiwater guard at
// the driver.
//
// `OriginalOccurredAt` is the original ArticleCreated timestamp (= the
// source row's `articles.created_at`) carried in RFC3339 form. Verraes'
// multi-temporal events pattern: the event's wall-clock OccurredAt
// records when the corrective event was emitted, while the payload's
// `original_occurred_at` records the fact-time when the article was
// first observed. Empty string is accepted (e.g. article rows whose
// source created_at is zero); future projectors may treat empty as
// "fact-time unknown" rather than rejecting.
type ArticleUrlBackfilledPayload struct {
	ArticleID          string `json:"article_id"`
	URL                string `json:"url"`
	OriginalOccurredAt string `json:"original_occurred_at"`
}
