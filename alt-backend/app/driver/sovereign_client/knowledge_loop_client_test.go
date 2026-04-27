package sovereign_client

import (
	"testing"
	"time"

	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProtoEntryToDomain_SurfacePlannerMetadataAndNarrativeWhyKinds(t *testing.T) {
	v2 := sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2
	pb := &sovereignv1.KnowledgeLoopEntry{
		UserId:                uuid.NewString(),
		TenantId:              uuid.NewString(),
		LensModeId:            "default",
		EntryKey:              "article:42",
		SourceItemKey:         "article:42",
		ProposedStage:         sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
		SurfaceBucket:         sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		ProjectionRevision:    1,
		ProjectionSeqHiwater:  100,
		SourceEventSeq:        100,
		FreshnessAt:           timestamppb.New(time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)),
		WhyPrimary:            &sovereignv1.KnowledgeLoopWhyPayload{Kind: sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY, Text: "topic match"},
		DismissState:          sovereignv1.DismissState_DISMISS_STATE_ACTIVE,
		RenderDepthHint:       2,
		LoopPriority:          sovereignv1.LoopPriority_LOOP_PRIORITY_CRITICAL,
		SurfacePlannerVersion: &v2,
		SurfaceScoreInputs:    []byte(`{"topic_overlap_count":1}`),
	}

	got, err := protoEntryToDomain(pb)
	require.NoError(t, err)
	require.Equal(t, domain.WhyKind("topic_affinity_why"), got.WhyKind)
	require.Equal(t, domain.SurfacePlannerV2, got.SurfacePlannerVersion)
	require.JSONEq(t, `{"topic_overlap_count":1}`, string(got.SurfaceScoreInputs))
}
