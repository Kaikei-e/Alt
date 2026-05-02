// Package knowledge_loop_projector hosts the projector responsible for turning
// knowledge_events rows into knowledge_loop_entries / knowledge_loop_session_state
// projection rows. The projector is reproject-safe: it depends only on event
// payload fields, never on latest projection state or wall-clock time.
//
// This is the canonical owner of Knowledge Loop projection logic. Earlier
// builds carried the same code in alt-backend behind an RPC hop; the migration
// (ADR-000844 follow-up) consolidates the responsibility into knowledge-sovereign
// so the projector reads events and writes projections in-process.
package knowledge_loop_projector

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// maxWhyTextBytes matches the canonical DB CHECK on knowledge_loop_entries.why_text.
const maxWhyTextBytes = 512

// maxEvidenceRefs matches the canonical contract §3.10 (evidence_refs length <= 8).
const maxEvidenceRefs = 8

// EnrichWhyFromEvent derives a structured WhyPayload from a knowledge_events row.
// The function is pure and reproject-safe: it reads event payload only (never
// latest projection state or time.Now()). Same event → same enrichment on replay.
//
// Canonical contract §11 (Why as first-class): the returned text is plain,
// 1..512 chars, no Markdown/HTML. evidence_refs are capped at 8 and stable
// (version ids, article ids, or the event id itself) so the UI can deep-link
// without fetching extra state.
func EnrichWhyFromEvent(ev *sovereign_db.KnowledgeEvent) *sovereignv1.KnowledgeLoopWhyPayload {
	payload := parseEnrichmentPayload(ev.Payload)

	switch ev.EventType {
	case EventSummaryVersionCreated:
		return enrichSummaryVersion(ev, payload)
	case EventSummaryNarrativeBackfilled:
		// Discovered event (ADR-000846) — same narrative shape as the original
		// SummaryVersionCreated path so the patch SQL writes a substantive
		// title-bearing why_text. Reusing enrichSummaryVersion guarantees a
		// stable text format across new emissions and future replay.
		return enrichSummaryVersion(ev, payload)
	case EventHomeItemsSeen:
		return enrichHomeItemsSeen(ev, payload)
	case EventHomeItemAsked:
		return enrichHomeItemAsked(ev, payload)
	case EventHomeItemOpened:
		return enrichHomeItemOpened(ev, payload)
	case EventHomeItemSuperseded, EventSummarySuperseded:
		return enrichSuperseded(ev, payload)
	case EventHomeItemDismissed:
		return enrichHomeItemDismissed(ev, payload)
	}
	// Fallback: keep entries renderable even for event types without dedicated
	// enrichment, so the projector never emits an empty why_text.
	return &sovereignv1.KnowledgeLoopWhyPayload{
		Kind: sovereignv1.WhyKind_WHY_KIND_SOURCE,
		Text: "A recent event surfaced this entry — open to see what changed.",
	}
}

// enrichmentPayload is the projection of event payload fields that the enricher
// consults. Each call decodes at most once; unknown fields are ignored.
type enrichmentPayload struct {
	SummaryVersionID       string
	TagSetVersionID        string
	LensVersionID          string
	ArticleID              string
	ArticleTitle           string
	ConversationID         string
	PreviousSummaryVersion string
	EntryKey               string
	NewEntryKey            string
	ActionType             string
	OpenedAt               string
}

func parseEnrichmentPayload(raw json.RawMessage) enrichmentPayload {
	out := enrichmentPayload{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	out.SummaryVersionID = readString(m, "summary_version_id")
	out.TagSetVersionID = readString(m, "tag_set_version_id")
	out.LensVersionID = readString(m, "lens_version_id")
	out.ArticleID = readString(m, "article_id")
	out.ArticleTitle = readString(m, "article_title")
	out.ConversationID = readString(m, "conversation_id")
	out.PreviousSummaryVersion = readString(m, "previous_summary_version", "old_summary_version_id")
	out.EntryKey = readString(m, "entry_key", "item_key")
	out.NewEntryKey = readString(m, "new_entry_key", "superseded_by_entry_key")
	out.ActionType = readString(m, "action_type", "action")
	out.OpenedAt = readString(m, "opened_at", "dismissed_at")
	return out
}

func readString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// --- per-event enrichment ---------------------------------------------------

// enrichSummaryVersion produces the source_why narrative for a fresh summary.
// The article title (when present in the event payload) is the most informative
// signal we can show; without it we fall back to a feed-oriented sentence so
// the card still reads as a real proposition rather than a placeholder.
func enrichSummaryVersion(_ *sovereign_db.KnowledgeEvent, p enrichmentPayload) *sovereignv1.KnowledgeLoopWhyPayload {
	var text string
	switch {
	case p.ArticleTitle != "":
		text = p.ArticleTitle + " — fresh summary ready to read."
	default:
		text = "A new summary is ready in one of your feeds."
	}
	return &sovereignv1.KnowledgeLoopWhyPayload{
		Kind:         sovereignv1.WhyKind_WHY_KIND_SOURCE,
		Text:         sanitizePlainText(text),
		EvidenceRefs: boundEvidence(appendRefIfPresent(nil, p.SummaryVersionID, "summary", p.ArticleID, "article", p.TagSetVersionID, "tags")),
	}
}

func enrichHomeItemsSeen(_ *sovereign_db.KnowledgeEvent, p enrichmentPayload) *sovereignv1.KnowledgeLoopWhyPayload {
	text := "Resurfaced from your home feed — pick up where you left off."
	if p.ArticleTitle != "" {
		text = p.ArticleTitle + " — back in your feed for a closer look."
	}
	return &sovereignv1.KnowledgeLoopWhyPayload{
		Kind:         sovereignv1.WhyKind_WHY_KIND_SOURCE,
		Text:         sanitizePlainText(text),
		EvidenceRefs: boundEvidence(appendRefIfPresent(nil, p.SummaryVersionID, "summary", p.TagSetVersionID, "tags", p.ArticleID, "article")),
	}
}

func enrichHomeItemAsked(_ *sovereign_db.KnowledgeEvent, p enrichmentPayload) *sovereignv1.KnowledgeLoopWhyPayload {
	text := "You started a conversation with Augur about this — pick the thread back up."
	if p.ArticleTitle != "" {
		text = p.ArticleTitle + " — your Augur thread is still open here."
	}
	refs := appendRefIfPresent(nil, p.ConversationID, "conversation", p.ArticleID, "article")
	return &sovereignv1.KnowledgeLoopWhyPayload{
		Kind:         sovereignv1.WhyKind_WHY_KIND_SOURCE,
		Text:         sanitizePlainText(text),
		EvidenceRefs: boundEvidence(refs),
	}
}

func enrichHomeItemOpened(ev *sovereign_db.KnowledgeEvent, p enrichmentPayload) *sovereignv1.KnowledgeLoopWhyPayload {
	text := "You opened this before — worth a second pass?"
	if p.ArticleTitle != "" {
		text = p.ArticleTitle + " — you opened this earlier; worth a second pass?"
	}
	// The open event itself is the stable anchor — reproject replays point at
	// the same event_id, so the UI can deep-link "last opened" to this row.
	refs := appendRefIfPresent(nil, ev.EventID.String(), "open_event", p.ArticleID, "article")
	return &sovereignv1.KnowledgeLoopWhyPayload{
		Kind:         sovereignv1.WhyKind_WHY_KIND_RECALL,
		Text:         sanitizePlainText(text),
		EvidenceRefs: boundEvidence(refs),
	}
}

func enrichSuperseded(_ *sovereign_db.KnowledgeEvent, p enrichmentPayload) *sovereignv1.KnowledgeLoopWhyPayload {
	text := "Updated since you last saw it — a newer version is available for review."
	if p.ArticleTitle != "" {
		text = p.ArticleTitle + " — a newer version replaces what you saw before."
	}
	refs := appendRefIfPresent(nil, p.PreviousSummaryVersion, "previous_summary", p.SummaryVersionID, "new_summary", p.EntryKey, "previous_entry", p.NewEntryKey, "new_entry")
	return &sovereignv1.KnowledgeLoopWhyPayload{
		Kind:         sovereignv1.WhyKind_WHY_KIND_CHANGE,
		Text:         sanitizePlainText(text),
		EvidenceRefs: boundEvidence(refs),
	}
}

func enrichHomeItemDismissed(ev *sovereign_db.KnowledgeEvent, p enrichmentPayload) *sovereignv1.KnowledgeLoopWhyPayload {
	when := p.OpenedAt
	if when == "" && ev != nil && !ev.OccurredAt.IsZero() {
		when = ev.OccurredAt.UTC().Format(time.RFC3339)
	}
	text := "Dismissed earlier — recheck, mark reviewed, or archive."
	if when != "" {
		text = "Dismissed at " + when + " — recheck, mark reviewed, or archive."
	}
	refs := appendRefIfPresent(nil, ev.EventID.String(), "dismiss_event", p.EntryKey, "entry")
	return &sovereignv1.KnowledgeLoopWhyPayload{
		Kind:         sovereignv1.WhyKind_WHY_KIND_SOURCE,
		Text:         sanitizePlainText(text),
		EvidenceRefs: boundEvidence(refs),
	}
}

// --- helpers ----------------------------------------------------------------

// appendRefIfPresent takes interleaved (refID, label) pairs and appends only
// those whose refID is non-empty.
func appendRefIfPresent(acc []*sovereignv1.KnowledgeLoopEvidenceRef, pairs ...string) []*sovereignv1.KnowledgeLoopEvidenceRef {
	for i := 0; i+1 < len(pairs); i += 2 {
		id, label := pairs[i], pairs[i+1]
		if id == "" {
			continue
		}
		acc = append(acc, &sovereignv1.KnowledgeLoopEvidenceRef{RefId: id, Label: label})
	}
	return acc
}

// boundEvidence enforces the ≤8 cap from the canonical contract.
func boundEvidence(refs []*sovereignv1.KnowledgeLoopEvidenceRef) []*sovereignv1.KnowledgeLoopEvidenceRef {
	if len(refs) > maxEvidenceRefs {
		return refs[:maxEvidenceRefs]
	}
	return refs
}

// sanitizePlainText strips obvious HTML/script markers and truncates to 512 bytes.
// It is not a full HTML sanitizer — the rule is that why_text is plain text and
// must not carry markup. Rejecting '<' entirely would drop inline code samples,
// so we remove recognised tag openers and collapse whitespace.
func sanitizePlainText(s string) string {
	s = stripAngleSpans(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "Surfaced from a recent event."
	}
	if len(s) <= maxWhyTextBytes {
		return s
	}
	trimmed := s[:maxWhyTextBytes]
	for !utf8.ValidString(trimmed) && len(trimmed) > 0 {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed
}

func stripAngleSpans(s string) string {
	var out strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}
