package knowledge_loop_projector

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func mkEvent(t *testing.T, articleTitle string) *sovereign_db.KnowledgeEvent {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"summary_version_id": "sv-1",
		"article_id":         "article:42",
		"article_title":      articleTitle,
	})
	require.NoError(t, err)
	uid := uuid.New()
	return &sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      1,
		OccurredAt:    time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &uid,
		EventType:     EventSummaryVersionCreated,
		AggregateType: "article",
		AggregateID:   "article:42",
		Payload:       body,
	}
}

// TestOverrideWhy_VersionDriftKeepsEnricherOutput pins that supersede /
// version_drift events keep the enricher's WHY_KIND_CHANGE narrative even
// when v2 evidence is also present — version drift is the strongest
// "what changed" signal in the contract.
func TestOverrideWhy_VersionDriftKeepsEnricherOutput(t *testing.T) {
	ev := mkEvent(t, "Version Drift")
	original := EnrichWhyFromEvent(ev)
	out := OverrideWhyFromSurfaceInputs(ev, original, SurfaceScoreInputs{
		VersionDriftCount: 1,
		HasAugurLink:      true, // would otherwise win — but version_drift outranks
		TopicOverlapCount: 5,
	})
	require.Equal(t, original.Kind, out.Kind, "version drift must not be downgraded")
	require.Equal(t, original.Text, out.Text)
}

// TestOverrideWhy_AugurLinkUpgradesToUnfinishedContinue pins the v3
// narrative for the HasAugurLink path. The enricher would have produced
// SOURCE; the override should re-stamp it as UNFINISHED_CONTINUE.
func TestOverrideWhy_AugurLinkUpgradesToUnfinishedContinue(t *testing.T) {
	ev := mkEvent(t, "Open Thread")
	in := OverrideWhyFromSurfaceInputs(ev, EnrichWhyFromEvent(ev), SurfaceScoreInputs{
		HasAugurLink: true,
	})
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE, in.Kind)
	require.Contains(t, in.Text, "Open Thread")
	require.Contains(t, strings.ToLower(in.Text), "augur")
}

// TestOverrideWhy_OpenInteractionPreservesEnricher pins that an open
// interaction (HomeItemOpened) keeps the enricher's RECALL output —
// the override is a no-op here because the enricher is already correct.
func TestOverrideWhy_OpenInteractionPreservesEnricher(t *testing.T) {
	uid := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:    uuid.New(),
		EventSeq:   1,
		OccurredAt: time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC),
		TenantID:   uuid.New(),
		UserID:     &uid,
		EventType:  EventHomeItemOpened,
		Payload:    json.RawMessage(`{"article_id":"article:42","article_title":"Recall Me"}`),
	}
	original := EnrichWhyFromEvent(ev)
	out := OverrideWhyFromSurfaceInputs(ev, original, SurfaceScoreInputs{
		HasOpenInteraction: true,
	})
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_RECALL, out.Kind)
	require.Equal(t, original.Text, out.Text)
}

// TestOverrideWhy_TopicOverlapUpgradesToTopicAffinity pins the v3 narrative
// for the topic_overlap_count path. Recap clusters that include the
// entry's terms are the canonical "connects to topics you've been
// reading about" signal.
func TestOverrideWhy_TopicOverlapUpgradesToTopicAffinity(t *testing.T) {
	ev := mkEvent(t, "Topic Overlap Match")
	out := OverrideWhyFromSurfaceInputs(ev, EnrichWhyFromEvent(ev), SurfaceScoreInputs{
		TopicOverlapCount: 3,
	})
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY, out.Kind)
	require.Contains(t, out.Text, "Topic Overlap Match")
	require.Contains(t, strings.ToLower(out.Text), "topics you")
}

// TestOverrideWhy_TagOverlapUpgradesToTagTrending pins the v3 narrative
// for the tag_overlap_count path. Tags rising across recent
// tag_set_versions are the canonical "tags you've followed are picking
// up here" signal.
func TestOverrideWhy_TagOverlapUpgradesToTagTrending(t *testing.T) {
	ev := mkEvent(t, "Tag Trend Match")
	out := OverrideWhyFromSurfaceInputs(ev, EnrichWhyFromEvent(ev), SurfaceScoreInputs{
		TagOverlapCount: 2,
	})
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING, out.Kind)
	require.Contains(t, out.Text, "Tag Trend Match")
	require.Contains(t, strings.ToLower(out.Text), "tags you")
}

// TestOverrideWhy_PriorityAugurBeatsTopic pins the priority order from
// the canonical contract: HasAugurLink outranks TopicOverlapCount. With
// both present, the result must be UNFINISHED_CONTINUE.
func TestOverrideWhy_PriorityAugurBeatsTopic(t *testing.T) {
	ev := mkEvent(t, "Priority Test")
	out := OverrideWhyFromSurfaceInputs(ev, EnrichWhyFromEvent(ev), SurfaceScoreInputs{
		HasAugurLink:      true,
		TopicOverlapCount: 5,
		TagOverlapCount:   5,
	})
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE, out.Kind)
}

// TestOverrideWhy_PriorityV9Ladder pins the v9 priority ladder
// (canonical contract §11): change > unfinished_continue > topic_affinity >
// tag_trending > recall > source. Each row asserts that the highest-ranked
// non-zero signal wins, even when lower-ranked signals are also present.
// This test enumerates the priority ordering changes Phase 3 introduced —
// most importantly that topic_affinity and tag_trending now beat recall
// (HasOpenInteraction), reversing the v8 order.
func TestOverrideWhy_PriorityV9Ladder(t *testing.T) {
	ev := mkEvent(t, "Priority Ladder")

	cases := []struct {
		name   string
		inputs SurfaceScoreInputs
		want   sovereignv1.WhyKind
	}{
		{
			name: "version_drift outranks everything else",
			inputs: SurfaceScoreInputs{
				VersionDriftCount:  1,
				HasAugurLink:       true,
				TopicOverlapCount:  5,
				TagOverlapCount:    5,
				HasOpenInteraction: true,
			},
			want: sovereignv1.WhyKind_WHY_KIND_CHANGE,
		},
		{
			name: "augur_link beats topic+tag+recall",
			inputs: SurfaceScoreInputs{
				HasAugurLink:       true,
				TopicOverlapCount:  5,
				TagOverlapCount:    5,
				HasOpenInteraction: true,
			},
			want: sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE,
		},
		{
			name: "topic_overlap beats tag_overlap and recall (v9 reversal)",
			inputs: SurfaceScoreInputs{
				TopicOverlapCount:  3,
				TagOverlapCount:    5,
				HasOpenInteraction: true,
			},
			want: sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY,
		},
		{
			name: "tag_overlap beats recall (v9 reversal)",
			inputs: SurfaceScoreInputs{
				TagOverlapCount:    2,
				HasOpenInteraction: true,
			},
			want: sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING,
		},
		{
			name: "recall wins only as residual kind",
			inputs: SurfaceScoreInputs{
				HasOpenInteraction: true,
			},
			want: sovereignv1.WhyKind_WHY_KIND_RECALL,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Each subcase needs an enricher output that lets us tell the
			// branches apart. SummarySuperseded produces CHANGE; HomeItemOpened
			// produces RECALL (preserved by override); other rows use the
			// SummaryVersionCreated default whose enricher output the override
			// re-stamps to TOPIC_AFFINITY / TAG_TRENDING / UNFINISHED_CONTINUE.
			seedEv := ev
			switch tc.want {
			case sovereignv1.WhyKind_WHY_KIND_CHANGE:
				uid := uuid.New()
				seedEv = &sovereign_db.KnowledgeEvent{
					EventID:    uuid.New(),
					EventSeq:   1,
					OccurredAt: time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC),
					TenantID:   uuid.New(),
					UserID:     &uid,
					EventType:  EventSummarySuperseded,
					Payload:    json.RawMessage(`{"article_id":"article:42","article_title":"Changed"}`),
				}
			case sovereignv1.WhyKind_WHY_KIND_RECALL:
				uid := uuid.New()
				seedEv = &sovereign_db.KnowledgeEvent{
					EventID:    uuid.New(),
					EventSeq:   1,
					OccurredAt: time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC),
					TenantID:   uuid.New(),
					UserID:     &uid,
					EventType:  EventHomeItemOpened,
					Payload:    json.RawMessage(`{"article_id":"article:42","article_title":"Recall Me"}`),
				}
			}
			out := OverrideWhyFromSurfaceInputs(seedEv, EnrichWhyFromEvent(seedEv), tc.inputs)
			require.Equal(t, tc.want, out.Kind, "v9 priority ladder")
		})
	}
}

// TestOverrideWhy_NoSignalsFallsBackToEnricher pins that with no v2
// signals the override is a no-op — same behaviour as v1.
func TestOverrideWhy_NoSignalsFallsBackToEnricher(t *testing.T) {
	ev := mkEvent(t, "Plain Source")
	original := EnrichWhyFromEvent(ev)
	out := OverrideWhyFromSurfaceInputs(ev, original, SurfaceScoreInputs{})
	require.Equal(t, original.Kind, out.Kind)
	require.Equal(t, original.Text, out.Text)
}

// TestOverrideWhy_FallbackTextWithoutTitle pins the no-title fallback
// narrative shape. Old replays whose payload omitted article_title get a
// non-empty narrative (the v3 strings still satisfy the 1..512 CHECK).
func TestOverrideWhy_FallbackTextWithoutTitle(t *testing.T) {
	uid := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventID:    uuid.New(),
		EventSeq:   1,
		OccurredAt: time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC),
		TenantID:   uuid.New(),
		UserID:     &uid,
		EventType:  EventSummaryVersionCreated,
		Payload:    json.RawMessage(`{}`), // no article_title
	}
	out := OverrideWhyFromSurfaceInputs(ev, EnrichWhyFromEvent(ev), SurfaceScoreInputs{
		TopicOverlapCount: 1,
	})
	require.Equal(t, sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY, out.Kind)
	require.NotEmpty(t, out.Text)
	require.LessOrEqual(t, len(out.Text), 512)
}
