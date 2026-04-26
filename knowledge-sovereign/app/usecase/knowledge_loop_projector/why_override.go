package knowledge_loop_projector

import (
	"google.golang.org/protobuf/proto"

	sovereign_db "knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// OverrideWhyFromSurfaceInputs adjusts the enricher-produced WhyPayload when
// Surface Planner v2's resolver returned non-zero v2 evidence (Wave 4-C
// narrative, ADR-000853). The function is **pure**: same inputs always
// produce the same output, so reproject yields the same Why text and kind
// for the same event.
//
// Priority order (canonical contract §11):
//
//  1. version_drift > 0 → keep enricher's WHY_KIND_CHANGE narrative
//     (the supersede signal is the strongest "what changed" hint).
//  2. augur_link → WHY_KIND_UNFINISHED_CONTINUE — the user has an open
//     conversation thread; the entry should pick that thread back up.
//  3. open_interaction → keep enricher's WHY_KIND_RECALL (single open).
//  4. topic_overlap > 0 → WHY_KIND_TOPIC_AFFINITY.
//  5. tag_overlap > 0 → WHY_KIND_TAG_TRENDING.
//  6. otherwise → return enricher output unchanged.
//
// The function only mutates Kind and Text. EvidenceRefs from the enricher
// are preserved so deep-link targets (article id, summary version, open
// event id, conversation id) remain stable across the override.
func OverrideWhyFromSurfaceInputs(
	ev *sovereign_db.KnowledgeEvent,
	why *sovereignv1.KnowledgeLoopWhyPayload,
	inputs SurfaceScoreInputs,
) *sovereignv1.KnowledgeLoopWhyPayload {
	if why == nil {
		return nil
	}
	// Defer to enricher when the supersede / change_why path has
	// already produced the strongest narrative. Don't downgrade.
	if inputs.VersionDriftCount > 0 {
		return why
	}
	if inputs.HasAugurLink {
		out := proto.Clone(why).(*sovereignv1.KnowledgeLoopWhyPayload)
		out.Kind = sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE
		out.Text = sanitizePlainText(unfinishedContinueNarrative(ev))
		return out
	}
	// HasOpenInteraction is already enricher's RECALL path; keep it.
	if inputs.HasOpenInteraction {
		return why
	}
	if inputs.TopicOverlapCount > 0 {
		out := proto.Clone(why).(*sovereignv1.KnowledgeLoopWhyPayload)
		out.Kind = sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY
		out.Text = sanitizePlainText(topicAffinityNarrative(ev))
		return out
	}
	if inputs.TagOverlapCount > 0 {
		out := proto.Clone(why).(*sovereignv1.KnowledgeLoopWhyPayload)
		out.Kind = sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING
		out.Text = sanitizePlainText(tagTrendingNarrative(ev))
		return out
	}
	return why
}

// Narrative templates pin the v3 phrasing that maps WhyKind → user-visible
// text. The {article_title} substitution falls back to a neutral sentence
// when the event payload omits it (older replays). Strings are kept short
// so the 512-byte why_text CHECK is never exceeded after sanitization.

func unfinishedContinueNarrative(ev *sovereign_db.KnowledgeEvent) string {
	p := parseEnrichmentPayload(ev.Payload)
	if p.ArticleTitle != "" {
		return p.ArticleTitle + " — pick up the Augur thread you opened earlier."
	}
	return "Pick up the Augur thread you opened earlier."
}

func topicAffinityNarrative(ev *sovereign_db.KnowledgeEvent) string {
	p := parseEnrichmentPayload(ev.Payload)
	if p.ArticleTitle != "" {
		return p.ArticleTitle + " — connects to topics you've been reading about."
	}
	return "Connects to topics you've been reading about."
}

func tagTrendingNarrative(ev *sovereign_db.KnowledgeEvent) string {
	p := parseEnrichmentPayload(ev.Payload)
	if p.ArticleTitle != "" {
		return p.ArticleTitle + " — tags you've followed recently are picking up here."
	}
	return "Tags you've followed recently are picking up here."
}
