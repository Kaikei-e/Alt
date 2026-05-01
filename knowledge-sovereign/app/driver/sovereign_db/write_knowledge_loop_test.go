package sovereign_db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func newLoopEntryProto(seq int64) *sovereignv1.KnowledgeLoopEntry {
	lensVer := "lens-v1"
	return &sovereignv1.KnowledgeLoopEntry{
		UserId:               uuid.New().String(),
		TenantId:             uuid.New().String(),
		LensModeId:           "default",
		EntryKey:             "article:42",
		SourceItemKey:        "article:42",
		ProposedStage:        sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
		SurfaceBucket:        sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		ProjectionSeqHiwater: seq,
		SourceEventSeq:       seq,
		FreshnessAt:          timestamppb.New(time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)),
		ArtifactVersionRef: &sovereignv1.KnowledgeLoopArtifactVersionRef{
			LensVersionId: &lensVer,
		},
		WhyPrimary: &sovereignv1.KnowledgeLoopWhyPayload{
			Kind: sovereignv1.WhyKind_WHY_KIND_SOURCE,
			Text: "new unread article",
		},
		DismissState:    sovereignv1.DismissState_DISMISS_STATE_ACTIVE,
		RenderDepthHint: 2,
		LoopPriority:    sovereignv1.LoopPriority_LOOP_PRIORITY_CRITICAL,
	}
}

// TestUpsertKnowledgeLoopEntry_Insert exercises the INSERT branch.
func TestUpsertKnowledgeLoopEntry_Insert(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	entry := newLoopEntryProto(100)

	anyArgs := make([]interface{}, 31)
	for i := range anyArgs {
		anyArgs[i] = pgxmock.AnyArg()
	}
	mock.ExpectQuery(regexp.QuoteMeta(upsertKnowledgeLoopEntryQuery)).
		WithArgs(anyArgs...).
		WillReturnRows(pgxmock.NewRows([]string{"projection_revision", "projection_seq_hiwater"}).
			AddRow(int64(1), int64(100)))

	res, err := repo.UpsertKnowledgeLoopEntry(context.Background(), entry)
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.False(t, res.SkippedBySeqHiwater)
	require.Equal(t, int64(1), res.ProjectionRevision)
	require.Equal(t, int64(100), res.ProjectionSeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertKnowledgeLoopEntry_WritesSurfacePlannerMetadata(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	entry := newLoopEntryProto(120)
	v2 := sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2
	entry.SurfacePlannerVersion = &v2
	entry.SurfaceScoreInputs = []byte(`{"topic_overlap_count":1,"event_type":"SummaryVersionCreated"}`)

	anyArgs := make([]interface{}, 31)
	for i := range anyArgs {
		anyArgs[i] = pgxmock.AnyArg()
	}
	anyArgs[29] = int16(2)
	anyArgs[30] = entry.SurfaceScoreInputs
	mock.ExpectQuery(regexp.QuoteMeta(upsertKnowledgeLoopEntryQuery)).
		WithArgs(anyArgs...).
		WillReturnRows(pgxmock.NewRows([]string{"projection_revision", "projection_seq_hiwater"}).
			AddRow(int64(1), int64(120)))

	res, err := repo.UpsertKnowledgeLoopEntry(context.Background(), entry)
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpsertKnowledgeLoopEntry_SeqHiwaterGuardSkipsStaleReplay verifies the seq-hiwater
// guard: an event with an older seq than the existing row's returns SkippedBySeqHiwater=true.
// This is the reproject-safety invariant — replaying historical events is a no-op when the
// projection has already advanced past them.
func TestUpsertKnowledgeLoopEntry_SeqHiwaterGuardSkipsStaleReplay(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	stale := newLoopEntryProto(50)

	anyArgs := make([]interface{}, 31)
	for i := range anyArgs {
		anyArgs[i] = pgxmock.AnyArg()
	}
	mock.ExpectQuery(regexp.QuoteMeta(upsertKnowledgeLoopEntryQuery)).
		WithArgs(anyArgs...).
		WillReturnError(pgx.ErrNoRows)

	res, err := repo.UpsertKnowledgeLoopEntry(context.Background(), stale)
	require.NoError(t, err, "stale seq must surface as SkippedBySeqHiwater, not as an error")
	require.False(t, res.Applied)
	require.True(t, res.SkippedBySeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPatchKnowledgeLoopEntryWhy_AppliesPatchOnly exercises the patch-only-why
// path (ADR-000846). The SQL must NOT touch dismiss_state, freshness_at,
// surface_bucket, or any other field — only the why_* columns. We assert the
// SQL text directly so a regression that re-introduces the full UPSERT
// columns (or accidentally writes dismiss_state) fails this test.
func TestPatchKnowledgeLoopEntryWhy_AppliesPatchOnly(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	why := &sovereignv1.KnowledgeLoopWhyPayload{
		Kind: sovereignv1.WhyKind_WHY_KIND_SOURCE,
		Text: "Article Title — fresh summary ready to read.",
		EvidenceRefs: []*sovereignv1.KnowledgeLoopEvidenceRef{
			{RefId: "sv-bf-1", Label: "summary"},
		},
	}

	mock.ExpectQuery(regexp.QuoteMeta(patchKnowledgeLoopEntryWhyQuery)).
		WithArgs(
			pgxmock.AnyArg(), // user_id
			pgxmock.AnyArg(), // tenant_id
			"default",        // lens_mode_id
			"article:42",     // entry_key
			int64(400),       // event_seq
			"source_why",     // why_kind
			why.Text,         // why_text
			pgxmock.AnyArg(), // why_confidence (nil)
			pgxmock.AnyArg(), // why_evidence_ref_ids
			pgxmock.AnyArg(), // why_evidence_refs JSONB
		).
		WillReturnRows(pgxmock.NewRows([]string{"projection_revision", "projection_seq_hiwater"}).
			AddRow(int64(2), int64(400)))

	res, err := repo.PatchKnowledgeLoopEntryWhy(
		context.Background(),
		uuid.New().String(),
		uuid.New().String(),
		"default",
		"article:42",
		400,
		why,
	)
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.False(t, res.SkippedBySeqHiwater)
	require.Equal(t, int64(2), res.ProjectionRevision)
	require.Equal(t, int64(400), res.ProjectionSeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())

	// The SQL string itself must not mention dismiss_state, freshness_at,
	// surface_bucket, proposed_stage, or other entry-level fields. This is
	// the structural guard that the patch path stays surgical.
	for _, forbidden := range []string{
		"dismiss_state",
		"freshness_at",
		"surface_bucket",
		"proposed_stage",
		"superseded_by_entry_key",
		"render_depth_hint",
		"loop_priority",
		"change_summary",
		"continue_context",
		"decision_options",
		"act_targets",
		"artifact_summary_version_id",
		"artifact_tag_set_version_id",
		"artifact_lens_version_id",
	} {
		require.NotContains(t, patchKnowledgeLoopEntryWhyQuery, forbidden,
			"patch SQL must NOT touch %s — that field belongs to the original "+
				"SummaryVersionCreated upsert path. ADR-000846 §Goal 2.", forbidden)
	}
}

// TestPatchKnowledgeLoopEntryWhy_SeqHiwaterGuardSkipsStaleReplay confirms that
// a missing entry OR a stale seq surfaces as SkippedBySeqHiwater rather than
// an error. Both outcomes are safe under reproject — the original
// SummaryVersionCreated event will rebuild the entry on its turn.
func TestPatchKnowledgeLoopEntryWhy_SeqHiwaterGuardSkipsStaleReplay(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	anyPatchArgs := make([]interface{}, 10)
	for i := range anyPatchArgs {
		anyPatchArgs[i] = pgxmock.AnyArg()
	}
	mock.ExpectQuery(regexp.QuoteMeta(patchKnowledgeLoopEntryWhyQuery)).
		WithArgs(anyPatchArgs...).
		WillReturnError(pgx.ErrNoRows)

	res, err := repo.PatchKnowledgeLoopEntryWhy(
		context.Background(),
		uuid.New().String(),
		uuid.New().String(),
		"default",
		"article:missing",
		50,
		&sovereignv1.KnowledgeLoopWhyPayload{
			Kind: sovereignv1.WhyKind_WHY_KIND_SOURCE,
			Text: "x",
		},
	)
	require.NoError(t, err)
	require.False(t, res.Applied)
	require.True(t, res.SkippedBySeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPatchKnowledgeLoopEntryDismissState_AppliesPatchOnly mirrors the why-only
// patch test (ADR-000846) for the canonical contract §8.2 Deferred path. The
// SQL must touch only lifecycle columns — every other entry-level field must
// remain untouched. A regression that broadens this UPDATE would reset the
// entry's freshness_at / why_text / decision_options from a stale event payload.
func TestPatchKnowledgeLoopEntryDismissState_AppliesPatchOnly(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	mock.ExpectQuery(regexp.QuoteMeta(patchKnowledgeLoopEntryDismissStateQuery)).
		WithArgs(
			pgxmock.AnyArg(), // user_id
			pgxmock.AnyArg(), // tenant_id
			"default",        // lens_mode_id
			"article:42",     // entry_key
			int64(600),       // event_seq
			"deferred",       // dismiss_state
			"snoozed",        // visibility_state
			"open",           // completion_state
		).
		WillReturnRows(pgxmock.NewRows([]string{"projection_revision", "projection_seq_hiwater"}).
			AddRow(int64(3), int64(600)))

	res, err := repo.PatchKnowledgeLoopEntryDismissState(
		context.Background(),
		uuid.New().String(),
		uuid.New().String(),
		"default",
		"article:42",
		600,
		sovereignv1.DismissState_DISMISS_STATE_DEFERRED,
	)
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.False(t, res.SkippedBySeqHiwater)
	require.Equal(t, int64(3), res.ProjectionRevision)
	require.Equal(t, int64(600), res.ProjectionSeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())

	// Structural guard: the SQL may touch only lifecycle columns. why_text /
	// freshness_at / surface_bucket / proposed_stage etc. belong to the
	// build-from-event upsert path and would clobber state the user has
	// already moved through.
	require.Contains(t, patchKnowledgeLoopEntryDismissStateQuery, "dismiss_state")
	require.Contains(t, patchKnowledgeLoopEntryDismissStateQuery, "visibility_state")
	require.Contains(t, patchKnowledgeLoopEntryDismissStateQuery, "completion_state")
	for _, forbidden := range []string{
		"why_kind",
		"why_text",
		"why_confidence",
		"why_evidence_ref_ids",
		"why_evidence_refs",
		"freshness_at",
		"surface_bucket",
		"proposed_stage",
		"superseded_by_entry_key",
		"render_depth_hint",
		"loop_priority",
		"change_summary",
		"continue_context",
		"decision_options",
		"act_targets",
		"artifact_summary_version_id",
		"artifact_tag_set_version_id",
		"artifact_lens_version_id",
	} {
		require.NotContains(t, patchKnowledgeLoopEntryDismissStateQuery, forbidden,
			"patch SQL must NOT touch %s — that field belongs to the build-from-event "+
				"upsert path. canonical-contract §8.2 / immutable-design-guard merge-safe.", forbidden)
	}
}

// TestPatchKnowledgeLoopEntryDismissState_SeqHiwaterGuardSkipsStaleReplay
// confirms that a missing entry or an out-of-order Deferred event surfaces as
// SkippedBySeqHiwater rather than an error. Both outcomes are safe under
// reproject — a later Deferred event will land normally on its turn.
func TestPatchKnowledgeLoopEntryDismissState_SeqHiwaterGuardSkipsStaleReplay(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	anyPatchArgs := make([]interface{}, 8)
	for i := range anyPatchArgs {
		anyPatchArgs[i] = pgxmock.AnyArg()
	}
	mock.ExpectQuery(regexp.QuoteMeta(patchKnowledgeLoopEntryDismissStateQuery)).
		WithArgs(anyPatchArgs...).
		WillReturnError(pgx.ErrNoRows)

	res, err := repo.PatchKnowledgeLoopEntryDismissState(
		context.Background(),
		uuid.New().String(),
		uuid.New().String(),
		"default",
		"article:missing",
		50,
		sovereignv1.DismissState_DISMISS_STATE_DEFERRED,
	)
	require.NoError(t, err)
	require.False(t, res.Applied)
	require.True(t, res.SkippedBySeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPatchKnowledgeLoopEntrySurfacePlan_AppliesPatchOnly pins the
// SurfacePlanRecomputed write contract. The event is a planner correction over
// an existing entry, so it may update only surface placement metadata and the
// seq/projection bookkeeping needed for replay safety.
func TestPatchKnowledgeLoopEntrySurfacePlan_AppliesPatchOnly(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	scoreInputs := []byte(`{"version_drift_count":1,"event_type":"knowledge_loop.surface_plan_recomputed.v1"}`)

	mock.ExpectQuery(regexp.QuoteMeta(patchKnowledgeLoopEntrySurfacePlanQuery)).
		WithArgs(
			pgxmock.AnyArg(), // user_id
			pgxmock.AnyArg(), // tenant_id
			"default",        // lens_mode_id
			"article:42",     // entry_key
			int64(950),       // event_seq
			"changed",        // surface_bucket
			int16(2),         // render_depth_hint
			"confirm",        // loop_priority
			int16(2),         // surface_planner_version
			scoreInputs,      // surface_score_inputs
		).
		WillReturnRows(pgxmock.NewRows([]string{"projection_revision", "projection_seq_hiwater"}).
			AddRow(int64(4), int64(950)))

	res, err := repo.PatchKnowledgeLoopEntrySurfacePlan(
		context.Background(),
		uuid.New().String(),
		uuid.New().String(),
		"default",
		"article:42",
		950,
		sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED,
		2,
		sovereignv1.LoopPriority_LOOP_PRIORITY_CONFIRM,
		sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2,
		scoreInputs,
	)
	require.NoError(t, err)
	require.True(t, res.Applied)
	require.False(t, res.SkippedBySeqHiwater)
	require.Equal(t, int64(4), res.ProjectionRevision)
	require.Equal(t, int64(950), res.ProjectionSeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Contains(t, patchKnowledgeLoopEntrySurfacePlanQuery, "surface_bucket")
	require.Contains(t, patchKnowledgeLoopEntrySurfacePlanQuery, "render_depth_hint")
	require.Contains(t, patchKnowledgeLoopEntrySurfacePlanQuery, "loop_priority")
	require.Contains(t, patchKnowledgeLoopEntrySurfacePlanQuery, "surface_planner_version")
	require.Contains(t, patchKnowledgeLoopEntrySurfacePlanQuery, "surface_score_inputs")
	for _, forbidden := range []string{
		"dismiss_state",
		"visibility_state",
		"completion_state",
		"why_kind",
		"why_text",
		"why_confidence",
		"why_evidence_ref_ids",
		"why_evidence_refs",
		"freshness_at",
		"source_observed_at",
		"proposed_stage",
		"superseded_by_entry_key",
		"change_summary",
		"continue_context",
		"decision_options",
		"act_targets",
		"artifact_summary_version_id",
		"artifact_tag_set_version_id",
		"artifact_lens_version_id",
	} {
		require.NotContains(t, patchKnowledgeLoopEntrySurfacePlanQuery, forbidden,
			"surface replan patch must not touch %s; it is a planner-only correction", forbidden)
	}
}

func TestPatchKnowledgeLoopEntrySurfacePlan_SeqHiwaterGuardSkipsStaleReplay(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	anyPatchArgs := make([]interface{}, 10)
	for i := range anyPatchArgs {
		anyPatchArgs[i] = pgxmock.AnyArg()
	}
	mock.ExpectQuery(regexp.QuoteMeta(patchKnowledgeLoopEntrySurfacePlanQuery)).
		WithArgs(anyPatchArgs...).
		WillReturnError(pgx.ErrNoRows)

	res, err := repo.PatchKnowledgeLoopEntrySurfacePlan(
		context.Background(),
		uuid.New().String(),
		uuid.New().String(),
		"default",
		"article:missing",
		50,
		sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		4,
		sovereignv1.LoopPriority_LOOP_PRIORITY_CRITICAL,
		sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2,
		[]byte(`{"topic_overlap_count":1}`),
	)
	require.NoError(t, err)
	require.False(t, res.Applied)
	require.True(t, res.SkippedBySeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertKnowledgeLoopEntrySessionState_AppliesSeqGuard(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	tenantID := uuid.New()
	enteredAt := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(upsertKnowledgeLoopEntrySessionStateQuery)).
		WithArgs(
			userID,
			tenantID,
			"default",
			"article:42",
			"orient",
			enteredAt,
			int64(700),
		).
		WillReturnRows(pgxmock.NewRows([]string{"projection_revision", "projection_seq_hiwater"}).
			AddRow(int64(1), int64(700)))

	res, err := repo.UpsertKnowledgeLoopEntrySessionState(
		context.Background(),
		userID.String(),
		tenantID.String(),
		"default",
		"article:42",
		sovereignv1.LoopStage_LOOP_STAGE_ORIENT,
		enteredAt,
		700,
	)

	require.NoError(t, err)
	require.True(t, res.Applied)
	require.False(t, res.SkippedBySeqHiwater)
	require.Equal(t, int64(700), res.ProjectionSeqHiwater)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertKnowledgeLoopSurface_AllowsClearingPrimary(t *testing.T) {
	require.Contains(t, upsertKnowledgeLoopSurfaceQuery, "primary_entry_key      = EXCLUDED.primary_entry_key",
		"surface recompute must be able to clear a stale primary when a bucket becomes empty")
	require.NotContains(t, upsertKnowledgeLoopSurfaceQuery, "COALESCE(EXCLUDED.primary_entry_key",
		"COALESCE would preserve stale primary_entry_key after archive/mark_reviewed")
}

// TestReserveKnowledgeLoopTransition_Fresh exercises a first-time idempotency claim.
func TestReserveKnowledgeLoopTransition_Fresh(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(reserveKnowledgeLoopTransitionQuery)).
		WithArgs(userID, "01JZA0000000000000000000000").
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(userID))

	res, err := repo.ReserveKnowledgeLoopTransition(context.Background(), userID, "01JZA0000000000000000000000")
	require.NoError(t, err)
	require.True(t, res.Reserved)
	require.Nil(t, res.CanonicalEntryKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestReserveKnowledgeLoopTransition_Duplicate verifies that a duplicate key returns the cached
// response so the caller can replay it without re-appending to knowledge_events.
func TestReserveKnowledgeLoopTransition_Duplicate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	clientTxID := "01JZA0000000000000000000001"
	canonical := "article:42"
	created := time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(reserveKnowledgeLoopTransitionQuery)).
		WithArgs(userID, clientTxID).
		WillReturnError(pgx.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(loadCachedKnowledgeLoopTransitionQuery)).
		WithArgs(userID, clientTxID).
		WillReturnRows(pgxmock.NewRows([]string{"canonical_entry_key", "response_payload", "created_at"}).
			AddRow(&canonical, []byte(`{"accepted":false}`), &created))

	res, err := repo.ReserveKnowledgeLoopTransition(context.Background(), userID, clientTxID)
	require.NoError(t, err)
	require.False(t, res.Reserved)
	require.NotNil(t, res.CanonicalEntryKey)
	require.Equal(t, "article:42", *res.CanonicalEntryKey)
	require.NoError(t, mock.ExpectationsWereMet())
}
