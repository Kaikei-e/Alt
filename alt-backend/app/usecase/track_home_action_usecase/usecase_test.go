package track_home_action_usecase

import (
	"alt/domain"
	"alt/port/knowledge_home_port"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUserEventPort implements knowledge_user_event_port.AppendKnowledgeUserEventPort.
type mockUserEventPort struct {
	appendedEvents []domain.KnowledgeUserEvent
	err            error
}

func (m *mockUserEventPort) AppendKnowledgeUserEvent(_ context.Context, event domain.KnowledgeUserEvent) error {
	if m.err != nil {
		return m.err
	}
	m.appendedEvents = append(m.appendedEvents, event)
	return nil
}

// mockKnowledgeEventPort implements knowledge_event_port.AppendKnowledgeEventPort.
type mockKnowledgeEventPort struct {
	appendedEvents []domain.KnowledgeEvent
	err            error
}

func (m *mockKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	m.appendedEvents = append(m.appendedEvents, event)
	return int64(len(m.appendedEvents)), nil
}

// mockRecallSignalPort implements recall_signal_port.AppendRecallSignalPort.
type mockRecallSignalPort struct {
	appendedSignals []domain.RecallSignal
	err             error
}

func (m *mockRecallSignalPort) AppendRecallSignal(_ context.Context, signal domain.RecallSignal) error {
	if m.err != nil {
		return m.err
	}
	m.appendedSignals = append(m.appendedSignals, signal)
	return nil
}

// mockFeatureFlagPort implements feature_flag_port.FeatureFlagPort.
type mockFeatureFlagPort struct {
	enabled bool
}

func (m *mockFeatureFlagPort) IsEnabled(_ string, _ uuid.UUID) bool {
	if m == nil {
		return true
	}
	return m.enabled
}

type mockDismissPort struct {
	calls []struct {
		userID            uuid.UUID
		itemKey           string
		projectionVersion int
		dismissedAt       time.Time
	}
	err error
}

func (m *mockDismissPort) DismissKnowledgeHomeItem(_ context.Context, userID uuid.UUID, itemKey string, projectionVersion int, dismissedAt time.Time) error {
	m.calls = append(m.calls, struct {
		userID            uuid.UUID
		itemKey           string
		projectionVersion int
		dismissedAt       time.Time
	}{
		userID:            userID,
		itemKey:           itemKey,
		projectionVersion: projectionVersion,
		dismissedAt:       dismissedAt,
	})
	return m.err
}

type mockActiveProjectionVersionPort struct {
	version *domain.KnowledgeProjectionVersion
	err     error
}

func (m *mockActiveProjectionVersionPort) GetActiveVersion(_ context.Context) (*domain.KnowledgeProjectionVersion, error) {
	return m.version, m.err
}

// flakyArticleURLLookupPort fails the first n calls then succeeds. Used to
// test the retry-on-transient-failure behaviour on the producer-side URL
// injection path (Auto-OODA suppression plan, Pillar 2C).
type flakyArticleURLLookupPort struct {
	calls      int
	failUntil  int
	succeedURL string
	err        error // returned for every failing call
}

func (m *flakyArticleURLLookupPort) LookupArticleURL(_ context.Context, _ string, _ uuid.UUID) (string, error) {
	m.calls++
	if m.calls <= m.failUntil {
		if m.err != nil {
			return "", m.err
		}
		return "", errors.New("transient lookup failure")
	}
	return m.succeedURL, nil
}

func TestTrackHomeActionUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()

	tests := []struct {
		name            string
		actionType      string
		itemKey         string
		metadataJSON    string
		userEventPort   *mockUserEventPort
		knowledgePort   *mockKnowledgeEventPort
		flagPort        *mockFeatureFlagPort
		wantErr         bool
		wantErrContains string
		wantUserEvents  int
		wantKnEvents    int
	}{
		{
			name:           "success - open action",
			actionType:     "open",
			itemKey:        "article:test-uuid",
			userEventPort:  &mockUserEventPort{},
			knowledgePort:  &mockKnowledgeEventPort{},
			flagPort:       nil,
			wantUserEvents: 1,
			wantKnEvents:   1,
		},
		{
			name:           "success - dismiss action",
			actionType:     "dismiss",
			itemKey:        "article:test-uuid",
			userEventPort:  &mockUserEventPort{},
			knowledgePort:  &mockKnowledgeEventPort{},
			flagPort:       nil,
			wantUserEvents: 1,
			wantKnEvents:   1,
		},
		{
			name:            "error - invalid action type",
			actionType:      "invalid",
			itemKey:         "article:test-uuid",
			userEventPort:   &mockUserEventPort{},
			knowledgePort:   &mockKnowledgeEventPort{},
			flagPort:        nil,
			wantErr:         true,
			wantErrContains: "invalid action type",
		},
		{
			name:            "error - empty item key",
			actionType:      "open",
			itemKey:         "",
			userEventPort:   &mockUserEventPort{},
			knowledgePort:   &mockKnowledgeEventPort{},
			flagPort:        nil,
			wantErr:         true,
			wantErrContains: "item_key is required",
		},
		{
			name:           "tracking disabled - no events",
			actionType:     "open",
			itemKey:        "article:test-uuid",
			userEventPort:  &mockUserEventPort{},
			knowledgePort:  &mockKnowledgeEventPort{},
			flagPort:       &mockFeatureFlagPort{enabled: false},
			wantUserEvents: 0,
			wantKnEvents:   0,
		},
		{
			name:           "tracking enabled - events recorded",
			actionType:     "open",
			itemKey:        "article:test-uuid",
			userEventPort:  &mockUserEventPort{},
			knowledgePort:  &mockKnowledgeEventPort{},
			flagPort:       &mockFeatureFlagPort{enabled: true},
			wantUserEvents: 1,
			wantKnEvents:   1,
		},
		{
			name:           "tracking disabled - dismiss still appends events",
			actionType:     "dismiss",
			itemKey:        "article:test-uuid",
			userEventPort:  &mockUserEventPort{},
			knowledgePort:  &mockKnowledgeEventPort{},
			flagPort:       &mockFeatureFlagPort{enabled: false},
			wantUserEvents: 1,
			wantKnEvents:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewTrackHomeActionUsecase(tt.userEventPort, tt.knowledgePort, tt.flagPort, nil, nil, nil, nil)
			err := uc.Execute(context.Background(), userID, tenantID, tt.actionType, tt.itemKey, tt.metadataJSON)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Len(t, tt.userEventPort.appendedEvents, tt.wantUserEvents)
			assert.Len(t, tt.knowledgePort.appendedEvents, tt.wantKnEvents)
		})
	}
}

func TestTrackHomeActionUsecase_RecallSignal(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()
	itemKey := "article:test-uuid"

	t.Run("open action appends SignalOpened", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, recallPort, nil, nil, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
		require.Len(t, recallPort.appendedSignals, 1)
		assert.Equal(t, domain.SignalOpened, recallPort.appendedSignals[0].SignalType)
		assert.Equal(t, itemKey, recallPort.appendedSignals[0].ItemKey)
		assert.Equal(t, userID, recallPort.appendedSignals[0].UserID)
	})

	t.Run("ask action appends SignalAugurReferenced", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, recallPort, nil, nil, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "ask", itemKey, "")
		require.NoError(t, err)
		require.Len(t, recallPort.appendedSignals, 1)
		assert.Equal(t, domain.SignalAugurReferenced, recallPort.appendedSignals[0].SignalType)
	})

	t.Run("listen action appends SignalTagInterest", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, recallPort, nil, nil, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "listen", itemKey, "")
		require.NoError(t, err)
		require.Len(t, recallPort.appendedSignals, 1)
		assert.Equal(t, domain.SignalTagInterest, recallPort.appendedSignals[0].SignalType)
	})

	t.Run("open_search action appends SignalSearchRelated", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, recallPort, nil, nil, nil)

		metadata := `{"search_query":"RAG pipeline"}`
		err := uc.Execute(context.Background(), userID, tenantID, "open_search", itemKey, metadata)
		require.NoError(t, err)
		require.Len(t, recallPort.appendedSignals, 1)
		assert.Equal(t, domain.SignalSearchRelated, recallPort.appendedSignals[0].SignalType)
		assert.Equal(t, itemKey, recallPort.appendedSignals[0].ItemKey)
	})

	t.Run("open_search action includes search_query in signal payload", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, recallPort, nil, nil, nil)

		metadata := `{"query":"rust async"}`
		err := uc.Execute(context.Background(), userID, tenantID, "open_search", itemKey, metadata)
		require.NoError(t, err)
		require.Len(t, recallPort.appendedSignals, 1)
		assert.Equal(t, "rust async", recallPort.appendedSignals[0].Payload["search_query"])
	})

	t.Run("dismiss action does not append signal", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, recallPort, nil, nil, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "dismiss", itemKey, "")
		require.NoError(t, err)
		assert.Empty(t, recallPort.appendedSignals)
	})

	t.Run("recall signal failure is non-fatal", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{err: errors.New("db error")}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, recallPort, nil, nil, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
	})

	t.Run("nil recall signal port does not panic", func(t *testing.T) {
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, nil, nil, nil, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
	})

	t.Run("tracking disabled skips signal generation", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, &mockFeatureFlagPort{enabled: false}, recallPort, nil, nil, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
		assert.Empty(t, recallPort.appendedSignals)
	})

	t.Run("tag_click action appends SignalTagClicked with tag in payload", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, recallPort, nil, nil, nil)

		metadata := `{"tag":"rust"}`
		err := uc.Execute(context.Background(), userID, tenantID, "tag_click", itemKey, metadata)
		require.NoError(t, err)
		require.Len(t, recallPort.appendedSignals, 1)
		assert.Equal(t, domain.SignalTagClicked, recallPort.appendedSignals[0].SignalType)
		assert.Equal(t, itemKey, recallPort.appendedSignals[0].ItemKey)
	})
}

func TestTrackHomeActionUsecase_DismissWriteThrough(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()
	itemKey := "article:test-uuid"

	t.Run("dismiss action updates read model synchronously", func(t *testing.T) {
		dismissPort := &mockDismissPort{}
		versionPort := &mockActiveProjectionVersionPort{
			version: &domain.KnowledgeProjectionVersion{Version: 7},
		}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, nil, dismissPort, versionPort, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "dismiss", itemKey, "")
		require.NoError(t, err)
		require.Len(t, dismissPort.calls, 1)
		assert.Equal(t, userID, dismissPort.calls[0].userID)
		assert.Equal(t, itemKey, dismissPort.calls[0].itemKey)
		assert.Equal(t, 7, dismissPort.calls[0].projectionVersion)
		assert.False(t, dismissPort.calls[0].dismissedAt.IsZero())
	})

	t.Run("dismiss read model failure is non fatal", func(t *testing.T) {
		dismissPort := &mockDismissPort{err: errors.New("db failed")}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, nil, dismissPort, &mockActiveProjectionVersionPort{}, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "dismiss", itemKey, "")
		require.NoError(t, err)
		require.Len(t, dismissPort.calls, 1)
	})

	t.Run("dismiss target not found is non fatal", func(t *testing.T) {
		dismissPort := &mockDismissPort{err: knowledge_home_port.ErrDismissTargetNotFound}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, nil, dismissPort, &mockActiveProjectionVersionPort{}, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "dismiss", itemKey, "")
		require.NoError(t, err)
		require.Len(t, dismissPort.calls, 1)
	})

	t.Run("non dismiss actions skip read model update", func(t *testing.T) {
		dismissPort := &mockDismissPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, nil, dismissPort, &mockActiveProjectionVersionPort{}, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
		assert.Empty(t, dismissPort.calls)
	})

	t.Run("dismiss falls back to default projection version when lookup fails", func(t *testing.T) {
		dismissPort := &mockDismissPort{}
		versionPort := &mockActiveProjectionVersionPort{err: errors.New("lookup failed")}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, nil, dismissPort, versionPort, nil)

		err := uc.Execute(context.Background(), userID, tenantID, "dismiss", itemKey, "")
		require.NoError(t, err)
		require.Len(t, dismissPort.calls, 1)
		assert.Equal(t, 1, dismissPort.calls[0].projectionVersion)
	})
}

func TestTrackHomeActionUsecase_ArticleURLRetry(t *testing.T) {
	// Auto-OODA / Open recoverable plan, Pillar 2C: producer-side LookupArticleURL
	// must retry on transient failure so URL injection succeeds when the article
	// row is briefly unavailable. Append-first invariant: the knowledge_event
	// MUST be appended even if every retry fails — the absent source_url is
	// recovered later by ArticleUrlBackfilled (immutable-design-guard PASS).
	logger.InitLogger()
	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()
	itemKey := "article:" + articleID.String()

	t.Run("succeeds on attempt 1: URL is injected, no extra retries", func(t *testing.T) {
		flaky := &flakyArticleURLLookupPort{failUntil: 0, succeedURL: "https://example.com/a"}
		knPort := &mockKnowledgeEventPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, knPort, nil, nil, nil, nil, flaky)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
		assert.Equal(t, 1, flaky.calls, "no retry on first-attempt success")
		require.Len(t, knPort.appendedEvents, 1)
		assert.Contains(t, string(knPort.appendedEvents[0].Payload), "https://example.com/a")
	})

	t.Run("succeeds on retry 2 of 3: URL is injected", func(t *testing.T) {
		flaky := &flakyArticleURLLookupPort{failUntil: 1, succeedURL: "https://example.com/b"}
		knPort := &mockKnowledgeEventPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, knPort, nil, nil, nil, nil, flaky)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
		assert.Equal(t, 2, flaky.calls, "must retry exactly until success")
		require.Len(t, knPort.appendedEvents, 1)
		assert.Contains(t, string(knPort.appendedEvents[0].Payload), "https://example.com/b")
	})

	t.Run("3 failures: event still appended with no URL (append-first)", func(t *testing.T) {
		// Append-first invariant (immutable-design-guard): URL lookup is best-
		// effort. Refusing to append when lookup fails would create silent
		// observation gaps in the event log. ArticleUrlBackfilled is the
		// long-term self-heal path.
		flaky := &flakyArticleURLLookupPort{failUntil: 3, succeedURL: "https://example.com/c"}
		knPort := &mockKnowledgeEventPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, knPort, nil, nil, nil, nil, flaky)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err, "lookup failure must not cascade")
		assert.Equal(t, 3, flaky.calls, "must attempt exactly 3 times")
		require.Len(t, knPort.appendedEvents, 1, "event MUST still be appended (append-first)")
		assert.NotContains(t, string(knPort.appendedEvents[0].Payload), "url",
			"payload must not carry an empty 'url' field; absence is the contract")
	})
}
