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
// Priority order (canonical contract §11, WhyMappingVersion v9):
//
//  1. version_drift > 0       → keep enricher's WHY_KIND_CHANGE narrative
//     (the supersede signal is the strongest
//     "what changed" hint and must not be
//     downgraded by overlap signals).
//  2. has_augur_link          → WHY_KIND_UNFINISHED_CONTINUE — the user has
//     an open conversation thread; the entry
//     should pick that thread back up.
//  3. topic_overlap_count > 0 → WHY_KIND_TOPIC_AFFINITY (recap-cluster
//     connection beats single-open recall).
//  4. tag_overlap_count > 0   → WHY_KIND_TAG_TRENDING.
//  5. has_open_interaction    → keep enricher's WHY_KIND_RECALL — RECALL
//     is now an evidence-of-last-resort kind:
//     a single open is a weaker connection than
//     an active topic / tag cluster the user is
//     currently following.
//  6. otherwise               → return enricher output unchanged.
//
// v9 change vs v8: RECALL was previously checked before topic / tag overlap.
// That order surfaced "you opened this once" as the why even when the entry
// also belonged to a hot recap cluster the user was actively reading. The
// v9 order surfaces the cluster connection instead, matching the canonical
// contract intent that RECALL is the residual kind. WhyMappingVersion is
// bumped to v9 so a full reproject converges to the new narrative.
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
	// 1. Defer to enricher when the supersede / change_why path has
	//    already produced the strongest narrative. Don't downgrade.
	if inputs.VersionDriftCount > 0 {
		return why
	}
	// 2. Augur thread continuation outranks all overlap signals because the
	//    user has an explicit, scoped intent (the open thread).
	if inputs.HasAugurLink {
		out := proto.Clone(why).(*sovereignv1.KnowledgeLoopWhyPayload)
		out.Kind = sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE
		out.Text = sanitizePlainText(unfinishedContinueNarrative(ev))
		refreshWhyV2OnKindChange(out)
		return out
	}
	// 3. Topic affinity (recap cluster overlap) outranks tag trending and
	//    recall because it captures a current-thinking connection.
	if inputs.TopicOverlapCount > 0 {
		out := proto.Clone(why).(*sovereignv1.KnowledgeLoopWhyPayload)
		out.Kind = sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY
		out.Text = sanitizePlainText(topicAffinityNarrative(ev))
		refreshWhyV2OnKindChange(out)
		return out
	}
	// 4. Tag trending — the user is following these tags as a stream.
	if inputs.TagOverlapCount > 0 {
		out := proto.Clone(why).(*sovereignv1.KnowledgeLoopWhyPayload)
		out.Kind = sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING
		out.Text = sanitizePlainText(tagTrendingNarrative(ev))
		refreshWhyV2OnKindChange(out)
		return out
	}
	// 5. RECALL is the residual evidence kind — a prior single open is the
	//    weakest "you've encountered this" signal and only wins when no
	//    cluster / tag overlap exists.
	if inputs.HasOpenInteraction {
		return why
	}
	return why
}

// refreshWhyV2OnKindChange recomputes confidence_ladder and
// what_would_change_my_mind from the post-override Kind. counter_evidence_refs
// from the enricher (typically only the supersede branch) are preserved.
// Reproject-safe: pure function of the post-override Kind.
func refreshWhyV2OnKindChange(out *sovereignv1.KnowledgeLoopWhyPayload) {
	if out == nil {
		return
	}
	ladder := confidenceLadderFromKind(out.Kind).ToProto()
	out.ConfidenceLadder = &ladder
	wwcm := whatWouldChangeFromKind(out.Kind)
	if wwcm != "" {
		out.WhatWouldChangeMyMind = &wwcm
	} else {
		out.WhatWouldChangeMyMind = nil
	}
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
