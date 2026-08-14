package job

import (
	"alt/domain"
	"alt/orchestrator/port/rag_integration_port"
	"alt/utils/logger"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recoveringRagIntegration fails its first `failures` upserts transiently and
// succeeds from then on: a rag-orchestrator outage that ends while the row is
// still inside its retry budget.
type recoveringRagIntegration struct {
	failures int
	err      error
	upserts  int
}

func (r *recoveringRagIntegration) RetrieveContext(_ context.Context, _ string, _ []string) ([]rag_integration_port.RagContext, error) {
	return nil, nil
}

func (r *recoveringRagIntegration) UpsertArticle(_ context.Context, _ rag_integration_port.UpsertArticleInput) error {
	r.upserts++
	if r.upserts <= r.failures {
		return r.err
	}
	return nil
}

func (r *recoveringRagIntegration) Answer(_ context.Context, _ rag_integration_port.AnswerInput) (<-chan string, error) {
	return nil, nil
}

// TestProcessOutboxEvents_RagUpsertLands_RefreshesTheAttemptBudget pins what
// maxOutboxUpsertAttempts is a budget *for*: outlasting one downstream service
// being redeployed. The row's two legs talk to two different services —
// rag-orchestrator and knowledge-sovereign — so spending the budget on an
// outage of the first and then charging the remainder to an outage of the
// second gives the second leg a window far shorter than the one the budget was
// sized for. In the extreme below it gets a single attempt, and the row is
// marked terminally FAILED on the very tick both of its side effects were
// finally making progress.
func TestProcessOutboxEvents_RagUpsertLands_RefreshesTheAttemptBudget(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	// rag-orchestrator is down for all but the last tick of the budget.
	rag := &recoveringRagIntegration{
		failures: maxOutboxUpsertAttempts - 1,
		err:      fmt.Errorf("%w: RAG UpsertIndex returned status 503", rag_integration_port.ErrRagUpsertTransient),
	}
	// knowledge-sovereign is down throughout.
	knowledgePort := &stubKnowledgeEventPort{err: fmt.Errorf("sovereign AppendKnowledgeEvent: unavailable")}
	retries := newOutboxRetryTracker()

	for tick := 1; tick < maxOutboxUpsertAttempts; tick++ {
		require.NoError(t, processOutboxEvents(context.Background(), repo, rag, knowledgePort, retries))
		require.Equal(t, "PENDING", repo.statusOf(eventID),
			"tick %d: the RAG outage is still inside the %d-attempt budget", tick, maxOutboxUpsertAttempts)
	}

	// The tick rag-orchestrator comes back: the upsert lands, the append does not.
	require.NoError(t, processOutboxEvents(context.Background(), repo, rag, knowledgePort, retries))
	require.Equal(t, maxOutboxUpsertAttempts, rag.upserts, "the last tick's upsert must have been attempted and succeeded")

	assert.Equal(t, "PENDING", repo.statusOf(eventID),
		"the RAG leg just delivered; the ArticleCreated leg must not inherit a budget the RAG outage already spent")
}

// TestProcessOutboxEvents_RefreshedAttemptBudget_IsStillBounded is the other
// half: refreshing the budget on progress is a reset, not an exemption. With
// the upsert delivered and knowledge-sovereign still down, the ArticleCreated
// leg spends its own budget and the row ends FAILED, rather than holding the
// head of the oldest-first claim while every newer article queues behind it.
func TestProcessOutboxEvents_RefreshedAttemptBudget_IsStillBounded(t *testing.T) {
	logger.InitLogger()

	eventID := uuid.New().String()
	repo := &mockOutboxRepo{events: []domain.OutboxEvent{outboxUpsertEventFixture(eventID)}}
	rag := &recoveringRagIntegration{
		failures: 1,
		err:      fmt.Errorf("%w: RAG UpsertIndex returned status 503", rag_integration_port.ErrRagUpsertTransient),
	}
	knowledgePort := &stubKnowledgeEventPort{err: fmt.Errorf("sovereign AppendKnowledgeEvent: unavailable")}
	retries := newOutboxRetryTracker()

	// One tick of RAG outage, so the shared counter is already non-zero when
	// the upsert lands on the next one.
	require.NoError(t, processOutboxEvents(context.Background(), repo, rag, knowledgePort, retries))
	require.Equal(t, "PENDING", repo.statusOf(eventID))

	for attempt := 1; attempt <= maxOutboxUpsertAttempts; attempt++ {
		require.NoError(t, processOutboxEvents(context.Background(), repo, rag, knowledgePort, retries))
		if attempt < maxOutboxUpsertAttempts {
			require.Equal(t, "PENDING", repo.statusOf(eventID),
				"ArticleCreated attempt %d of %d must still be retried", attempt, maxOutboxUpsertAttempts)
		}
	}

	assert.Equal(t, "FAILED", repo.statusOf(eventID),
		"a sovereign outage outliving the ArticleCreated leg's own budget must still end the row")
	assert.Equal(t, 2, rag.upserts,
		"the delivered upsert must not be re-run once per tick while only the append is failing")
}
