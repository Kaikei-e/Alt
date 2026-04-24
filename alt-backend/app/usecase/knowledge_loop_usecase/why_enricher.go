package knowledge_loop_usecase

import (
	"alt/domain"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// maxWhyTextBytes matches the canonical DB CHECK on knowledge_loop_entries.why_text.
const maxWhyTextBytes = 512

// maxEvidenceRefs matches the canonical contract §3.10 (evidence_refs length <= 8).
const maxEvidenceRefs = 8

// EnrichedWhy is the in-process shape the projector uses to populate the Why
// fields of a KnowledgeLoopEntry. It is a superset of what the proto exposes:
// domain.WhyKind → proto WhyKind; plain-text Text; and a bounded EvidenceRef slice.
type EnrichedWhy struct {
	Kind         domain.WhyKind
	Text         string
	EvidenceRefs []domain.EvidenceRef
}

// EnrichWhyFromEvent derives a structured WhyPayload from a knowledge_events row.
// The function is pure and reproject-safe: it reads event payload only (never
// latest projection state or time.Now()). Same event → same enrichment on replay.
//
// Canonical contract §11 (Why as first-class): the returned text is plain,
// 1..512 chars, no Markdown/HTML. evidence_refs are capped at 8 and stable
// (version ids, article ids, or the event id itself) so the UI can deep-link
// without fetching extra state.
func EnrichWhyFromEvent(ev *domain.KnowledgeEvent) EnrichedWhy {
	payload := parseEnrichmentPayload(ev.Payload)

	switch ev.EventType {
	case domain.EventSummaryVersionCreated:
		return enrichSummaryVersion(ev, payload)
	case domain.EventHomeItemsSeen:
		return enrichHomeItemsSeen(ev, payload)
	case domain.EventHomeItemAsked:
		return enrichHomeItemAsked(ev, payload)
	case domain.EventHomeItemOpened:
		return enrichHomeItemOpened(ev, payload)
	case domain.EventHomeItemSuperseded, domain.EventSummarySuperseded:
		return enrichSuperseded(ev, payload)
	case domain.EventHomeItemDismissed:
		return EnrichedWhy{
			Kind: domain.WhyKindSource,
			Text: "You dismissed this item.",
		}
	}
	// Fallback: keep entries renderable even for event types without dedicated
	// enrichment, so the projector never emits an empty why_text.
	return EnrichedWhy{
		Kind: domain.WhyKindSource,
		Text: "Surfaced from a recent event.",
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

func enrichSummaryVersion(_ *domain.KnowledgeEvent, p enrichmentPayload) EnrichedWhy {
	var sb strings.Builder
	sb.WriteString("New summary")
	if p.ArticleTitle != "" {
		sb.WriteString(": ")
		sb.WriteString(p.ArticleTitle)
	}
	return EnrichedWhy{
		Kind:         domain.WhyKindSource,
		Text:         sanitizePlainText(sb.String()),
		EvidenceRefs: boundEvidence(appendRefIfPresent(nil, p.SummaryVersionID, "summary", p.ArticleID, "article", p.TagSetVersionID, "tags")),
	}
}

func enrichHomeItemsSeen(_ *domain.KnowledgeEvent, p enrichmentPayload) EnrichedWhy {
	return EnrichedWhy{
		Kind:         domain.WhyKindSource,
		Text:         "Surfaced in your home feed.",
		EvidenceRefs: boundEvidence(appendRefIfPresent(nil, p.SummaryVersionID, "summary", p.TagSetVersionID, "tags", p.ArticleID, "article")),
	}
}

func enrichHomeItemAsked(_ *domain.KnowledgeEvent, p enrichmentPayload) EnrichedWhy {
	refs := appendRefIfPresent(nil, p.ConversationID, "conversation", p.ArticleID, "article")
	return EnrichedWhy{
		Kind:         domain.WhyKindSource,
		Text:         "You asked about this item.",
		EvidenceRefs: boundEvidence(refs),
	}
}

func enrichHomeItemOpened(ev *domain.KnowledgeEvent, p enrichmentPayload) EnrichedWhy {
	// The open event itself is the stable anchor — reproject replays point at
	// the same event_id, so the UI can deep-link "last opened" to this row.
	refs := appendRefIfPresent(nil, ev.EventID.String(), "open_event", p.ArticleID, "article")
	return EnrichedWhy{
		Kind:         domain.WhyKindRecall,
		Text:         "You opened this before.",
		EvidenceRefs: boundEvidence(refs),
	}
}

func enrichSuperseded(_ *domain.KnowledgeEvent, p enrichmentPayload) EnrichedWhy {
	refs := appendRefIfPresent(nil, p.PreviousSummaryVersion, "previous_summary", p.SummaryVersionID, "new_summary", p.EntryKey, "previous_entry", p.NewEntryKey, "new_entry")
	return EnrichedWhy{
		Kind:         domain.WhyKindChange,
		Text:         "A newer version is available.",
		EvidenceRefs: boundEvidence(refs),
	}
}

// --- helpers ---------------------------------------------------------------

// appendRefIfPresent takes interleaved (refID, label) pairs and appends only
// those whose refID is non-empty. Centralizing this keeps the per-event
// functions readable and eliminates a class of typos.
func appendRefIfPresent(acc []domain.EvidenceRef, pairs ...string) []domain.EvidenceRef {
	for i := 0; i+1 < len(pairs); i += 2 {
		id, label := pairs[i], pairs[i+1]
		if id == "" {
			continue
		}
		acc = append(acc, domain.EvidenceRef{RefID: id, Label: label})
	}
	return acc
}

// boundEvidence enforces the ≤8 cap from the canonical contract.
func boundEvidence(refs []domain.EvidenceRef) []domain.EvidenceRef {
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
	// Remove angle-bracketed spans so scripted payloads do not round-trip.
	s = stripAngleSpans(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "Surfaced from a recent event."
	}
	// Truncate on rune boundaries so we never split a multibyte character.
	if len(s) <= maxWhyTextBytes {
		return s
	}
	// Walk backwards until we land on a safe rune boundary at or below the cap.
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
