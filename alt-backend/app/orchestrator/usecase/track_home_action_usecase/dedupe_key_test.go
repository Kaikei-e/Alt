package track_home_action_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dedupingUserEventPort models what sovereign actually does with dedupe_key
// instead of recording every call: the column gates a partial unique index
// conditioned on a non-empty key, so an append carrying a key the registry has
// already seen is an ON CONFLICT DO NOTHING no-op, not a new row. Counting rows
// here answers the question the user actually has — "did my retry leave a
// duplicate behind?" — rather than pinning the shape of the key.
type dedupingUserEventPort struct {
	rows []domain.KnowledgeUserEvent
	seen map[string]bool
}

func (m *dedupingUserEventPort) AppendKnowledgeUserEvent(_ context.Context, event domain.KnowledgeUserEvent) error {
	if m.seen == nil {
		m.seen = make(map[string]bool)
	}
	if event.DedupeKey != "" && m.seen[event.DedupeKey] {
		return nil
	}
	m.seen[event.DedupeKey] = true
	m.rows = append(m.rows, event)
	return nil
}

// dedupingKnowledgeEventPort is the knowledge_events half of the same model.
// err is settable between calls so a test can fail one attempt and let the
// retry through, which is exactly the sequence the fatal append created.
type dedupingKnowledgeEventPort struct {
	rows []domain.KnowledgeEvent
	seen map[string]bool
	err  error
}

func (m *dedupingKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if m.seen == nil {
		m.seen = make(map[string]bool)
	}
	if event.DedupeKey != "" && m.seen[event.DedupeKey] {
		return 0, nil
	}
	m.seen[event.DedupeKey] = true
	m.rows = append(m.rows, event)
	return int64(len(m.rows)), nil
}

func TestTrackHomeActionUsecase_RetryAfterFatalAppendDoesNotDuplicate(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()
	const itemKey = "article:test-uuid"

	// The knowledge_events append is fatal, so Execute can return an error with
	// the user event already committed, and the client's only recourse is to
	// re-issue the same action. That retry has to land on the same dedupe key:
	// append-first buys idempotency from the dedupe registry, not from the
	// caller, so a key that moves between attempts stacks a second user event
	// on top of the first and the guarantee is gone.
	userPort := &dedupingUserEventPort{}
	knPort := &dedupingKnowledgeEventPort{err: errors.New("sovereign unavailable")}
	uc := NewTrackHomeActionUsecase(userPort, knPort, nil, nil, nil, nil, nil)

	require.Error(t, uc.Execute(context.Background(), userID, tenantID, "dismiss", itemKey, ""))
	require.Len(t, userPort.rows, 1, "the user event lands before the append that fails")

	// A retry is issued some milliseconds later — never inside the millisecond
	// the first attempt happened to occupy.
	time.Sleep(5 * time.Millisecond)
	knPort.err = nil

	require.NoError(t, uc.Execute(context.Background(), userID, tenantID, "dismiss", itemKey, ""))

	assert.Len(t, userPort.rows, 1,
		"retrying one user action must not leave a second user event behind")
	assert.Len(t, knPort.rows, 1,
		"the retry is the first knowledge event for this action, and the only one")
}

func TestTrackHomeActionUsecase_DedupeKeySurvivesRetryWithClientKey(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()
	const itemKey = "article:test-uuid"

	// metadata_json is the only channel a client has for a retry-stable key
	// without a proto change, so an idempotency_key found there decides the
	// dedupe key outright.
	t.Run("same idempotency_key collapses the retry", func(t *testing.T) {
		userPort := &dedupingUserEventPort{}
		uc := NewTrackHomeActionUsecase(userPort, &dedupingKnowledgeEventPort{}, nil, nil, nil, nil, nil)
		const metadata = `{"idempotency_key":"action-42"}`

		require.NoError(t, uc.Execute(context.Background(), userID, tenantID, "open", itemKey, metadata))
		time.Sleep(5 * time.Millisecond)
		require.NoError(t, uc.Execute(context.Background(), userID, tenantID, "open", itemKey, metadata))

		assert.Len(t, userPort.rows, 1, "a client that supplies a key gets exact idempotency")
	})

	t.Run("distinct idempotency_keys keep two deliberate opens apart", func(t *testing.T) {
		userPort := &dedupingUserEventPort{}
		uc := NewTrackHomeActionUsecase(userPort, &dedupingKnowledgeEventPort{}, nil, nil, nil, nil, nil)

		require.NoError(t, uc.Execute(context.Background(), userID, tenantID, "open", itemKey, `{"idempotency_key":"a"}`))
		require.NoError(t, uc.Execute(context.Background(), userID, tenantID, "open", itemKey, `{"idempotency_key":"b"}`))

		assert.Len(t, userPort.rows, 2, "two keys means the client meant two actions")
	})
}

func TestTrackHomeActionUsecase_DistinctActionsInOneBucketStayDistinct(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()
	const itemKey = "article:test-uuid"

	// Clicking "rust" and then "go" on one item seconds apart is two facts, not
	// a retry. Quantising the clock coarsely enough to recognise a retry must
	// not swallow them, so the metadata has to reach the key too.
	userPort := &dedupingUserEventPort{}
	uc := NewTrackHomeActionUsecase(userPort, &dedupingKnowledgeEventPort{}, nil, nil, nil, nil, nil)

	require.NoError(t, uc.Execute(context.Background(), userID, tenantID, "tag_click", itemKey, `{"tag":"rust"}`))
	require.NoError(t, uc.Execute(context.Background(), userID, tenantID, "tag_click", itemKey, `{"tag":"go"}`))

	assert.Len(t, userPort.rows, 2,
		"two different tags clicked on one item are two events, not one deduped away")
}
