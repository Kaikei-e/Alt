package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRepo implements ReadDB (MutationRepository + ReadOperations).
type mockRepo struct {
	lastMethod      string
	lastPayload     json.RawMessage
	returnErr       error
	returnLens      *sovereign_db.KnowledgeLens
	returnVersion   *sovereign_db.KnowledgeLensVersion
	trailFootprints []sovereign_db.TrailFootprint
	gotTrailCursor  string
	gotTrailLimit   int

	anchorBranches   []sovereign_db.TrailBranch
	gotAnchorUserID  uuid.UUID
	gotAnchorItemKey string
	gotAnchorLimit   int
}

// --- MutationRepository ---
func (m *mockRepo) UpsertKnowledgeHomeItem(_ context.Context, p json.RawMessage) error {
	m.lastMethod = "UpsertKnowledgeHomeItem"
	m.lastPayload = p
	return m.returnErr
}
func (m *mockRepo) DismissKnowledgeHomeItem(_ context.Context, p json.RawMessage) error {
	m.lastMethod = "DismissKnowledgeHomeItem"
	m.lastPayload = p
	return m.returnErr
}
func (m *mockRepo) ClearSupersedeState(_ context.Context, p json.RawMessage) error {
	m.lastMethod = "ClearSupersedeState"
	m.lastPayload = p
	return m.returnErr
}
func (m *mockRepo) UpsertTodayDigest(_ context.Context, p json.RawMessage) error {
	m.lastMethod = "UpsertTodayDigest"
	m.lastPayload = p
	return m.returnErr
}
func (m *mockRepo) UpsertRecallCandidate(_ context.Context, p json.RawMessage) error {
	m.lastMethod = "UpsertRecallCandidate"
	m.lastPayload = p
	return m.returnErr
}
func (m *mockRepo) SnoozeRecallCandidate(_ context.Context, p json.RawMessage) error {
	m.lastMethod = "SnoozeRecallCandidate"
	m.lastPayload = p
	return m.returnErr
}
func (m *mockRepo) DismissRecallCandidate(_ context.Context, p json.RawMessage) error {
	m.lastMethod = "DismissRecallCandidate"
	m.lastPayload = p
	return m.returnErr
}
func (m *mockRepo) PatchKnowledgeHomeItemURL(_ context.Context, p json.RawMessage) error {
	m.lastMethod = "PatchKnowledgeHomeItemURL"
	m.lastPayload = p
	return m.returnErr
}

// --- ReadOperations stubs (return empty results) ---
func (m *mockRepo) GetKnowledgeHomeItems(_ context.Context, _ uuid.UUID, _ string, _ int, _ *sovereign_db.LensFilter) ([]sovereign_db.KnowledgeHomeItem, string, bool, error) {
	return nil, "", false, m.returnErr
}
func (m *mockRepo) GetTrailFootprints(_ context.Context, _ uuid.UUID, cursor string, limit int, _ []string) ([]sovereign_db.TrailFootprint, string, bool, error) {
	m.gotTrailCursor = cursor
	m.gotTrailLimit = limit
	return m.trailFootprints, "", false, m.returnErr
}
func (m *mockRepo) GetOpenTrailBranches(_ context.Context, _ uuid.UUID) ([]sovereign_db.TrailBranch, error) {
	return nil, m.returnErr
}
func (m *mockRepo) GetOpenTrailBranchesForAnchor(_ context.Context, userID uuid.UUID, anchorItemKey string, limit int) ([]sovereign_db.TrailBranch, error) {
	m.gotAnchorUserID = userID
	m.gotAnchorItemKey = anchorItemKey
	m.gotAnchorLimit = limit
	return m.anchorBranches, m.returnErr
}
func (m *mockRepo) GetTodayDigest(_ context.Context, _ uuid.UUID, _ time.Time) (*sovereign_db.TodayDigest, error) {
	return nil, m.returnErr
}
func (m *mockRepo) GetRecallCandidates(_ context.Context, _ uuid.UUID, _ int) ([]sovereign_db.RecallCandidate, error) {
	return nil, m.returnErr
}
func (m *mockRepo) ListDistinctUserIDs(_ context.Context) ([]uuid.UUID, error) {
	return nil, m.returnErr
}
func (m *mockRepo) CountNeedToKnowItems(_ context.Context, _ uuid.UUID, _ time.Time) (int, error) {
	return 0, m.returnErr
}
func (m *mockRepo) GetProjectionFreshness(_ context.Context, _ string) (*time.Time, error) {
	return nil, m.returnErr
}
func (m *mockRepo) ListKnowledgeEventsSince(_ context.Context, _ int64, _ int) ([]sovereign_db.KnowledgeEvent, error) {
	return nil, m.returnErr
}
func (m *mockRepo) ListKnowledgeEventsSinceForUser(_ context.Context, _, _ uuid.UUID, _ int64, _ int) ([]sovereign_db.KnowledgeEvent, error) {
	return nil, m.returnErr
}
func (m *mockRepo) GetLatestKnowledgeEventSeqForUser(_ context.Context, _, _ uuid.UUID) (int64, error) {
	return 0, m.returnErr
}
func (m *mockRepo) AppendKnowledgeEvent(_ context.Context, _ sovereign_db.KnowledgeEvent) (int64, error) {
	return 0, m.returnErr
}
func (m *mockRepo) AreArticlesVisibleInLens(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID, _ *sovereign_db.LensFilter) (map[uuid.UUID]bool, error) {
	return nil, m.returnErr
}
func (m *mockRepo) GetActiveProjectionVersion(_ context.Context) (*sovereign_db.ProjectionVersion, error) {
	return nil, m.returnErr
}
func (m *mockRepo) ListProjectionVersions(_ context.Context) ([]sovereign_db.ProjectionVersion, error) {
	return nil, m.returnErr
}
func (m *mockRepo) CreateProjectionVersion(_ context.Context, _ sovereign_db.ProjectionVersion) error {
	return m.returnErr
}
func (m *mockRepo) ActivateProjectionVersion(_ context.Context, _ int) error {
	return m.returnErr
}
func (m *mockRepo) GetProjectionCheckpoint(_ context.Context, _ string) (int64, error) {
	return 0, m.returnErr
}
func (m *mockRepo) UpdateProjectionCheckpoint(_ context.Context, _ string, _ int64) error {
	return m.returnErr
}
func (m *mockRepo) GetProjectionLag(_ context.Context) (float64, error) { return 0, m.returnErr }
func (m *mockRepo) GetProjectionAge(_ context.Context) (float64, error) { return 0, m.returnErr }
func (m *mockRepo) GetReprojectRun(_ context.Context, _ uuid.UUID) (*sovereign_db.ReprojectRun, error) {
	return nil, m.returnErr
}
func (m *mockRepo) ListReprojectRuns(_ context.Context, _ string, _ int) ([]sovereign_db.ReprojectRun, error) {
	return nil, m.returnErr
}
func (m *mockRepo) CreateReprojectRun(_ context.Context, _ sovereign_db.ReprojectRun) error {
	return m.returnErr
}
func (m *mockRepo) UpdateReprojectRun(_ context.Context, _ sovereign_db.ReprojectRun) error {
	return m.returnErr
}
func (m *mockRepo) CompareProjections(_ context.Context, _, _ string) (*sovereign_db.ReprojectDiffSummary, error) {
	return &sovereign_db.ReprojectDiffSummary{}, m.returnErr
}
func (m *mockRepo) ListProjectionAudits(_ context.Context, _ string, _ int) ([]sovereign_db.ProjectionAudit, error) {
	return nil, m.returnErr
}
func (m *mockRepo) CreateProjectionAudit(_ context.Context, _ sovereign_db.ProjectionAudit) error {
	return m.returnErr
}
func (m *mockRepo) GetBackfillJob(_ context.Context, _ uuid.UUID) (*sovereign_db.BackfillJob, error) {
	return nil, m.returnErr
}
func (m *mockRepo) ListBackfillJobs(_ context.Context) ([]sovereign_db.BackfillJob, error) {
	return nil, m.returnErr
}
func (m *mockRepo) CreateBackfillJob(_ context.Context, _ sovereign_db.BackfillJob) error {
	return m.returnErr
}
func (m *mockRepo) UpdateBackfillJob(_ context.Context, _ sovereign_db.BackfillJob) error {
	return m.returnErr
}
func (m *mockRepo) ListLenses(_ context.Context, _ uuid.UUID) ([]sovereign_db.KnowledgeLens, error) {
	return nil, m.returnErr
}
func (m *mockRepo) GetLens(_ context.Context, _ uuid.UUID) (*sovereign_db.KnowledgeLens, error) {
	return m.returnLens, m.returnErr
}
func (m *mockRepo) GetCurrentLensVersion(_ context.Context, _ uuid.UUID) (*sovereign_db.KnowledgeLensVersion, error) {
	return m.returnVersion, m.returnErr
}
func (m *mockRepo) GetCurrentLensSelection(_ context.Context, _ uuid.UUID) (*sovereign_db.KnowledgeCurrentLens, error) {
	return nil, m.returnErr
}
func (m *mockRepo) ResolveLensFilter(_ context.Context, _ uuid.UUID, _ *uuid.UUID) (*sovereign_db.LensFilter, error) {
	return nil, m.returnErr
}
func (m *mockRepo) CreateLens(_ context.Context, _ sovereign_db.KnowledgeLens) error {
	return m.returnErr
}
func (m *mockRepo) CreateLensVersion(_ context.Context, _ sovereign_db.KnowledgeLensVersion) error {
	return m.returnErr
}
func (m *mockRepo) SelectCurrentLens(_ context.Context, _ sovereign_db.KnowledgeCurrentLens) error {
	return m.returnErr
}
func (m *mockRepo) ClearCurrentLens(_ context.Context, _ uuid.UUID) error { return m.returnErr }
func (m *mockRepo) ArchiveLens(_ context.Context, _ uuid.UUID) error      { return m.returnErr }
func (m *mockRepo) ListRecallSignalsByUser(_ context.Context, _ uuid.UUID, _ int) ([]sovereign_db.RecallSignal, error) {
	return nil, m.returnErr
}
func (m *mockRepo) AppendRecallSignal(_ context.Context, _ sovereign_db.RecallSignal) error {
	return m.returnErr
}
func (m *mockRepo) AppendKnowledgeUserEvent(_ context.Context, _ sovereign_db.KnowledgeUserEvent) error {
	m.lastMethod = "AppendKnowledgeUserEvent"
	return m.returnErr
}

func setupTestServer(repo ReadDB) (sovereignv1connect.KnowledgeSovereignServiceClient, func()) {
	h := NewSovereignHandler(repo)
	mux := http.NewServeMux()
	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(h)
	mux.Handle(path, rpcHandler)
	srv := httptest.NewServer(mux)
	client := sovereignv1connect.NewKnowledgeSovereignServiceClient(srv.Client(), srv.URL)
	return client, srv.Close
}

func TestApplyProjectionMutation_DispatchesCorrectly(t *testing.T) {
	tests := []struct {
		mutationType   string
		expectedMethod string
	}{
		{MutationUpsertHomeItem, "UpsertKnowledgeHomeItem"},
		{MutationDismissHomeItem, "DismissKnowledgeHomeItem"},
		{MutationClearSupersede, "ClearSupersedeState"},
		{MutationUpsertTodayDigest, "UpsertTodayDigest"},
		{MutationUpsertRecallCandidate, "UpsertRecallCandidate"},
	}

	for _, tc := range tests {
		t.Run(tc.mutationType, func(t *testing.T) {
			repo := &mockRepo{}
			client, cleanup := setupTestServer(repo)
			defer cleanup()

			resp, err := client.ApplyProjectionMutation(context.Background(),
				connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
					MutationType:   tc.mutationType,
					EntityId:       "test-entity",
					Payload:        []byte(`{}`),
					IdempotencyKey: "key-1",
				}))

			require.NoError(t, err)
			assert.True(t, resp.Msg.Success)
			assert.Equal(t, tc.expectedMethod, repo.lastMethod)
		})
	}
}

func TestApplyProjectionMutation_UnknownType(t *testing.T) {
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	_, err := client.ApplyProjectionMutation(context.Background(),
		connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
			MutationType: "unknown_type",
		}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown projection mutation type")
}

func TestApplyProjectionMutation_RepoError(t *testing.T) {
	repo := &mockRepo{returnErr: fmt.Errorf("db error")}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	_, err := client.ApplyProjectionMutation(context.Background(),
		connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
			MutationType: MutationUpsertHomeItem,
			Payload:      []byte(`{}`),
		}))

	require.Error(t, err)
}

func TestApplyRecallMutation_DispatchesCorrectly(t *testing.T) {
	tests := []struct {
		mutationType   string
		expectedMethod string
	}{
		{MutationUpsertCandidate, "UpsertRecallCandidate"},
		{MutationSnoozeCandidate, "SnoozeRecallCandidate"},
		{MutationDismissCandidate, "DismissRecallCandidate"},
	}

	for _, tc := range tests {
		t.Run(tc.mutationType, func(t *testing.T) {
			repo := &mockRepo{}
			client, cleanup := setupTestServer(repo)
			defer cleanup()

			resp, err := client.ApplyRecallMutation(context.Background(),
				connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
					MutationType: tc.mutationType,
					Payload:      []byte(`{}`),
				}))

			require.NoError(t, err)
			assert.True(t, resp.Msg.Success)
			assert.Equal(t, tc.expectedMethod, repo.lastMethod)
		})
	}
}

func TestApplyCurationMutation_DismissCuration(t *testing.T) {
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	resp, err := client.ApplyCurationMutation(context.Background(),
		connect.NewRequest(&sovereignv1.ApplyCurationMutationRequest{
			MutationType: MutationDismissCuration,
			Payload:      []byte(`{}`),
		}))

	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
	assert.Equal(t, "DismissKnowledgeHomeItem", repo.lastMethod)
}

func TestApplyCurationMutation_UnknownTypeRejected(t *testing.T) {
	// Lens mutations (create_lens, select_lens, ...) are dispatched through
	// their own dedicated RPCs (CreateLens, SelectCurrentLens, ...) in
	// rpc_lens.go, never through ApplyCurationMutation. A mutation_type this
	// handler does not recognize — whether a stale lens-mutation label, a
	// producer typo, or an unimplemented type — must be rejected loudly
	// (Rule 8), not silently ACKed as success.
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	resp, err := client.ApplyCurationMutation(context.Background(),
		connect.NewRequest(&sovereignv1.ApplyCurationMutationRequest{
			MutationType: "create_lens",
			EntityId:     "lens-123",
			Payload:      []byte(`{"name":"test"}`),
		}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Nil(t, resp)
	assert.Equal(t, "", repo.lastMethod)
}
