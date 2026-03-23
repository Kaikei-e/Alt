package alt_db

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ApplyProjectionMutation dispatch ---

func TestApplyProjectionMutation_UnknownType(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown projection mutation type")
}

func TestApplyProjectionMutation_InvalidPayload(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationUpsertHomeItem,
		Payload:      []byte(`{invalid`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

// --- ApplyRecallMutation dispatch ---

func TestApplyRecallMutation_UnknownType(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{
		MutationType: "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown recall mutation type")
}

// --- ApplyCurationMutation dispatch ---

func TestApplyCurationMutation_UnknownType(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{
		MutationType: "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown curation mutation type")
}

// --- ApplyProjectionMutation: DismissHomeItem dispatch ---

func TestApplyProjectionMutation_DismissHomeItem_Dispatches(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)

	mock.ExpectExec("UPDATE knowledge_home_items").
		WithArgs(now, userID, "article:test-123", 2).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	payload, _ := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"item_key":           "article:test-123",
		"projection_version": 2,
		"dismissed_at":       now.Format(time.RFC3339Nano),
	})
	err = repo.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationDismissHomeItem,
		EntityID:     "article:test-123",
		Payload:      payload,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- ApplyProjectionMutation: ClearSupersede dispatch ---

func TestApplyProjectionMutation_ClearSupersede_Dispatches(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()

	mock.ExpectExec("UPDATE knowledge_home_items").
		WithArgs(userID, "article:test-456", 1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	payload, _ := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"item_key":           "article:test-456",
		"projection_version": 1,
	})
	err = repo.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationClearSupersede,
		EntityID:     "article:test-456",
		Payload:      payload,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- ApplyRecallMutation: SnoozeCandidate dispatch ---

func TestApplyRecallMutation_SnoozeCandidate_Dispatches(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	until := time.Now().Add(24 * time.Hour).Truncate(time.Microsecond)

	mock.ExpectExec("UPDATE recall_candidate_view").
		WithArgs(until, userID, "article:test-789").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	payload, _ := json.Marshal(map[string]any{
		"user_id":  userID.String(),
		"item_key": "article:test-789",
		"until":    until.Format(time.RFC3339Nano),
	})
	err = repo.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{
		MutationType: knowledge_sovereign_port.MutationSnoozeCandidate,
		EntityID:     "article:test-789",
		Payload:      payload,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- ApplyRecallMutation: DismissCandidate dispatch ---

func TestApplyRecallMutation_DismissCandidate_Dispatches(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()

	mock.ExpectExec("DELETE FROM recall_candidate_view").
		WithArgs(userID, "article:test-abc").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	payload, _ := json.Marshal(map[string]any{
		"user_id":  userID.String(),
		"item_key": "article:test-abc",
	})
	err = repo.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{
		MutationType: knowledge_sovereign_port.MutationDismissCandidate,
		EntityID:     "article:test-abc",
		Payload:      payload,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- ApplyCurationMutation: DismissCuration dispatch ---

func TestApplyCurationMutation_DismissCuration_Dispatches(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()

	// DismissCuration の payload は user_id と item_key のみ（TrackHomeActionUsecase が送る形式）。
	// driver は projection_version=1, dismissed_at=now() をデフォルトで使う。
	mock.ExpectExec("UPDATE knowledge_home_items").
		WithArgs(pgxmock.AnyArg(), userID, "article:test-def", 1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	payload, _ := json.Marshal(map[string]any{
		"user_id":  userID.String(),
		"item_key": "article:test-def",
	})
	err = repo.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{
		MutationType: knowledge_sovereign_port.MutationDismissCuration,
		EntityID:     "article:test-def",
		Payload:      payload,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- ResolveRetentionDecision ---

func TestResolveRetentionDecision_KnownEntityType(t *testing.T) {
	repo := &AltDBRepository{}
	policy, err := repo.ResolveRetentionDecision(context.Background(), "article_metadata", "article-123")
	require.NoError(t, err)
	assert.Equal(t, domain.RetentionTierHot, policy.Tier)
	assert.Equal(t, "article_metadata", policy.EntityType)
}

func TestResolveRetentionDecision_UnknownEntityType(t *testing.T) {
	repo := &AltDBRepository{}
	_, err := repo.ResolveRetentionDecision(context.Background(), "unknown_type", "id-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown entity type")
}

// --- ResolveExportScope ---

func TestResolveExportScope_KnownEntityType(t *testing.T) {
	repo := &AltDBRepository{}
	cls, err := repo.ResolveExportScope(context.Background(), "feed_subscriptions", "sub-123")
	require.NoError(t, err)
	assert.Equal(t, domain.ExportTierA, cls.Tier)
	assert.Equal(t, "feed_subscriptions", cls.EntityType)
}

func TestResolveExportScope_UnknownEntityType(t *testing.T) {
	repo := &AltDBRepository{}
	_, err := repo.ResolveExportScope(context.Background(), "unknown_type", "id-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown entity type")
}
