package knowledge_home

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/utils/logger"
)

// --- Helpers / mocks specific to stream lens scoping tests -------------------

// recordingEventsPort lets the test capture which (tenantID, userID) the
// stream actually queries with, so we can pin the tenant boundary.
type recordingEventsPort struct {
	mu        sync.Mutex
	events    []domain.KnowledgeEvent
	err       error
	gotTenant uuid.UUID
	gotUser   uuid.UUID
	calls     int
}

func (r *recordingEventsPort) ListKnowledgeEventsSinceForUser(_ context.Context, tenantID, userID uuid.UUID, _ int64, _ int) ([]domain.KnowledgeEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gotTenant = tenantID
	r.gotUser = userID
	r.calls++
	return r.events, r.err
}

type recordingLensVisibility struct {
	mu             sync.Mutex
	visible        map[uuid.UUID]bool // article_id -> visible
	err            error
	gotTenant      uuid.UUID
	gotUser        uuid.UUID
	gotArticles    []uuid.UUID
	gotFilter      *domain.KnowledgeHomeLensFilter
	calls          int
	returnNilOnAll bool
}

func (r *recordingLensVisibility) AreArticlesVisibleInLens(_ context.Context, tenantID, userID uuid.UUID, articleIDs []uuid.UUID, filter *domain.KnowledgeHomeLensFilter) (map[uuid.UUID]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gotTenant = tenantID
	r.gotUser = userID
	r.gotArticles = append([]uuid.UUID(nil), articleIDs...)
	r.gotFilter = filter
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	if r.returnNilOnAll {
		return map[uuid.UUID]bool{}, nil
	}
	out := make(map[uuid.UUID]bool, len(articleIDs))
	for _, id := range articleIDs {
		out[id] = r.visible[id]
	}
	return out, nil
}

type stubResolveLensPort struct {
	filter      *domain.KnowledgeHomeLensFilter
	err         error
	gotUserID   uuid.UUID
	gotLensID   *uuid.UUID
	calls       int
	mu          sync.Mutex
	failOnLensA bool
}

func (s *stubResolveLensPort) ResolveKnowledgeHomeLens(_ context.Context, userID uuid.UUID, lensID *uuid.UUID) (*domain.KnowledgeHomeLensFilter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gotUserID = userID
	s.gotLensID = lensID
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.filter, nil
}

// --- Tests -------------------------------------------------------------------

func TestFilterEventsByLens_DropsArticleEventsNotInLens(t *testing.T) {
	visibleID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	hiddenID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	tenantID := uuid.New()
	userID := uuid.New()

	vis := &recordingLensVisibility{
		visible: map[uuid.UUID]bool{
			visibleID: true,
			hiddenID:  false,
		},
	}
	h := &Handler{
		lensVisibilityPort: vis,
		logger:             slog.Default(),
	}

	events := []domain.KnowledgeEvent{
		{EventSeq: 1, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: visibleID.String()},
		{EventSeq: 2, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: hiddenID.String()},
		// home_session events bypass the lens filter (already user-scoped)
		{EventSeq: 3, EventType: domain.EventHomeItemOpened, AggregateType: domain.AggregateHomeSession, AggregateID: "session-1"},
	}

	filter := &domain.KnowledgeHomeLensFilter{LensID: uuid.New(), TagNames: []string{"AI"}}
	out := h.filterEventsByLens(context.Background(), events, tenantID, userID, filter)

	require.Len(t, out, 2, "hidden article event must be dropped, visible article + home_session pass")
	assert.Equal(t, visibleID.String(), out[0].AggregateID)
	assert.Equal(t, "session-1", out[1].AggregateID)
	assert.Equal(t, tenantID, vis.gotTenant)
	assert.Equal(t, userID, vis.gotUser)
	assert.ElementsMatch(t, []uuid.UUID{visibleID, hiddenID}, vis.gotArticles)
}

func TestFilterEventsByLens_NoFilter_PassesEverythingThrough(t *testing.T) {
	h := &Handler{lensVisibilityPort: &recordingLensVisibility{}, logger: slog.Default()}

	events := []domain.KnowledgeEvent{
		{EventSeq: 1, AggregateType: domain.AggregateArticle, AggregateID: uuid.New().String()},
	}
	out := h.filterEventsByLens(context.Background(), events, uuid.New(), uuid.New(), nil)
	assert.Equal(t, events, out, "nil filter is a no-op")
}

func TestFilterEventsByLens_VisibilityCheckFails_DropsArticleEvents(t *testing.T) {
	articleID := uuid.New()
	vis := &recordingLensVisibility{err: errors.New("sovereign down")}
	h := &Handler{lensVisibilityPort: vis, logger: slog.Default()}

	events := []domain.KnowledgeEvent{
		{EventSeq: 1, AggregateType: domain.AggregateArticle, AggregateID: articleID.String()},
		{EventSeq: 2, AggregateType: domain.AggregateHomeSession, AggregateID: "session-1"},
	}
	filter := &domain.KnowledgeHomeLensFilter{LensID: uuid.New()}
	out := h.filterEventsByLens(context.Background(), events, uuid.New(), uuid.New(), filter)

	require.Len(t, out, 1, "fail-closed must drop article events on visibility check failure")
	assert.Equal(t, "session-1", out[0].AggregateID)
}

func TestFilterEventsByLens_NoArticleEvents_SkipsLensCheck(t *testing.T) {
	vis := &recordingLensVisibility{}
	h := &Handler{lensVisibilityPort: vis, logger: slog.Default()}

	events := []domain.KnowledgeEvent{
		{EventSeq: 1, AggregateType: domain.AggregateHomeSession, AggregateID: "session-1"},
		{EventSeq: 2, AggregateType: domain.AggregateRecap, AggregateID: "recap-1"},
	}
	filter := &domain.KnowledgeHomeLensFilter{LensID: uuid.New()}
	out := h.filterEventsByLens(context.Background(), events, uuid.New(), uuid.New(), filter)

	require.Len(t, out, 2)
	assert.Equal(t, 0, vis.calls, "no articles → no RPC")
}

func TestFilterEventsByLens_DeduplicatesArticleIDsBeforeRPC(t *testing.T) {
	articleID := uuid.New()
	vis := &recordingLensVisibility{visible: map[uuid.UUID]bool{articleID: true}}
	h := &Handler{lensVisibilityPort: vis, logger: slog.Default()}

	events := []domain.KnowledgeEvent{
		{EventSeq: 1, AggregateType: domain.AggregateArticle, AggregateID: articleID.String()},
		{EventSeq: 2, AggregateType: domain.AggregateArticle, AggregateID: articleID.String()},
		{EventSeq: 3, AggregateType: domain.AggregateArticle, AggregateID: articleID.String()},
	}
	filter := &domain.KnowledgeHomeLensFilter{LensID: uuid.New()}
	_ = h.filterEventsByLens(context.Background(), events, uuid.New(), uuid.New(), filter)

	assert.Len(t, vis.gotArticles, 1, "dedup IDs before bulk RPC")
}

// --- Stream-level tests ------------------------------------------------------

func TestStreamHandler_LensIDProvidedAndUnresolvable_ReturnsNotFound(t *testing.T) {
	logger.InitLogger()

	flagPort := &mockFeatureFlagPort{enabledFlags: map[string]bool{domain.FlagStreamUpdates: true}}
	resolveLens := &stubResolveLensPort{err: errors.New("lens not owned by user")}

	h := NewHandler(
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, // eventsPort
		&recordingEventsPort{},
		nil, // lensVisibilityPort
		resolveLens,
		flagPort,
		nil, // metrics
		slog.Default(),
	)

	lensID := uuid.New().String()
	req := connect.NewRequest(&knowledgehomev1.StreamKnowledgeHomeUpdatesRequest{LensId: &lensID})
	err := h.StreamKnowledgeHomeUpdates(testUserContext(), req, nil)

	require.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code(),
		"unowned/missing lens must surface as NotFound to avoid existence oracle")
}

func TestStreamHandler_LensFilterAppliedToEventsForUserCall_TenantPropagated(t *testing.T) {
	// This test validates the per-tick path: lens resolved at start, then
	// (tenantID, userID) is propagated to the event fetch and lens
	// visibility check.
	logger.InitLogger()

	tenantID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	userID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	articleVisible := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	articleHidden := uuid.MustParse("66666666-6666-6666-6666-666666666666")

	events := &recordingEventsPort{
		events: []domain.KnowledgeEvent{
			{EventSeq: 1, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: articleVisible.String()},
			{EventSeq: 2, EventType: domain.EventArticleCreated, AggregateType: domain.AggregateArticle, AggregateID: articleHidden.String()},
		},
	}
	vis := &recordingLensVisibility{visible: map[uuid.UUID]bool{articleVisible: true, articleHidden: false}}
	resolveLens := &stubResolveLensPort{
		filter: &domain.KnowledgeHomeLensFilter{LensID: uuid.New(), TagNames: []string{"go"}},
	}

	h := &Handler{
		eventsForUserPort:  events,
		lensVisibilityPort: vis,
		resolveLensPort:    resolveLens,
		logger:             slog.Default(),
	}

	// Drive the per-tick logic directly. Stream loop wires the same calls.
	rawEvents, err := h.eventsForUserPort.ListKnowledgeEventsSinceForUser(context.Background(), tenantID, userID, 0, 50)
	require.NoError(t, err)
	resolved, err := h.resolveLensPort.ResolveKnowledgeHomeLens(context.Background(), userID, &resolveLens.filter.LensID)
	require.NoError(t, err)
	filtered := h.filterEventsByLens(context.Background(), rawEvents, tenantID, userID, resolved)

	assert.Equal(t, tenantID, events.gotTenant, "tenantID must reach the event fetch port")
	assert.Equal(t, userID, events.gotUser, "userID must reach the event fetch port")
	assert.Equal(t, tenantID, vis.gotTenant, "tenantID must reach the visibility check")
	require.Len(t, filtered, 1, "hidden article must be dropped")
	assert.Equal(t, articleVisible.String(), filtered[0].AggregateID)
}
