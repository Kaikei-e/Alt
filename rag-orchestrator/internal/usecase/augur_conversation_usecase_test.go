package usecase

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAugurRepo mirrors the postgres repo contract: append-only message log,
// conversation rows are write-once, summaries are derived from messages.
type fakeAugurRepo struct {
	mu       sync.Mutex
	convs    map[uuid.UUID]domain.AugurConversation
	messages map[uuid.UUID][]domain.AugurMessage
}

func newFakeAugurRepo() *fakeAugurRepo {
	return &fakeAugurRepo{
		convs:    map[uuid.UUID]domain.AugurConversation{},
		messages: map[uuid.UUID][]domain.AugurMessage{},
	}
}

func (f *fakeAugurRepo) CreateConversation(_ context.Context, c *domain.AugurConversation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.convs[c.ID] = *c
	return nil
}

func (f *fakeAugurRepo) GetConversation(_ context.Context, id uuid.UUID, userID uuid.UUID) (*domain.AugurConversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.convs[id]
	if !ok || c.UserID != userID {
		return nil, nil
	}
	cc := c
	return &cc, nil
}

func (f *fakeAugurRepo) AppendMessage(_ context.Context, msg *domain.AugurMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages[msg.ConversationID] = append(f.messages[msg.ConversationID], *msg)
	return nil
}

func (f *fakeAugurRepo) ListMessages(_ context.Context, conversationID uuid.UUID) ([]domain.AugurMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := append([]domain.AugurMessage{}, f.messages[conversationID]...)
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].CreatedAt.Before(msgs[j].CreatedAt) })
	return msgs, nil
}

func (f *fakeAugurRepo) ListSummaries(
	_ context.Context,
	userID uuid.UUID,
	limit int,
	afterActivity *time.Time,
	afterID *uuid.UUID,
) ([]domain.AugurConversationSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	summaries := []domain.AugurConversationSummary{}
	for _, c := range f.convs {
		if c.UserID != userID {
			continue
		}
		msgs := f.messages[c.ID]
		last := c.CreatedAt
		preview := ""
		if len(msgs) > 0 {
			last = msgs[len(msgs)-1].CreatedAt
			preview = msgs[len(msgs)-1].Content
			if len(preview) > 140 {
				preview = preview[:140]
			}
		}
		summaries = append(summaries, domain.AugurConversationSummary{
			ID:                 c.ID,
			UserID:             c.UserID,
			Title:              c.Title,
			CreatedAt:          c.CreatedAt,
			LastActivityAt:     last,
			MessageCount:       len(msgs),
			LastMessagePreview: preview,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if !summaries[i].LastActivityAt.Equal(summaries[j].LastActivityAt) {
			return summaries[i].LastActivityAt.After(summaries[j].LastActivityAt)
		}
		return summaries[i].ID.String() > summaries[j].ID.String()
	})
	if afterActivity != nil && afterID != nil {
		cut := -1
		for i, s := range summaries {
			if s.LastActivityAt.Before(*afterActivity) ||
				(s.LastActivityAt.Equal(*afterActivity) && s.ID.String() < afterID.String()) {
				cut = i
				break
			}
		}
		if cut < 0 {
			return nil, nil
		}
		summaries = summaries[cut:]
	}
	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, nil
}

func (f *fakeAugurRepo) DeleteConversation(_ context.Context, id uuid.UUID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.convs[id]; ok && c.UserID == userID {
		delete(f.convs, id)
		delete(f.messages, id)
	}
	return nil
}

func fixedClock(base time.Time) (func() time.Time, func()) {
	t := base
	return func() time.Time {
			now := t
			t = t.Add(time.Second) // each call advances by one second
			return now
		}, func() {}
}

func TestEnsureConversation_MintsNewWhenIDIsZero(t *testing.T) {
	repo := newFakeAugurRepo()
	clock, _ := fixedClock(time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC))
	uc := NewAugurConversationUsecase(repo, clock)

	userID := uuid.New()
	conv, err := uc.EnsureConversation(context.Background(), userID, uuid.Nil, "  What's the weather    like today?\n")
	require.NoError(t, err)
	require.NotNil(t, conv)
	assert.Equal(t, userID, conv.UserID)
	assert.Equal(t, "What's the weather like today?", conv.Title)
	assert.NotEqual(t, uuid.Nil, conv.ID)
}

func TestEnsureConversation_TruncatesLongTitles(t *testing.T) {
	repo := newFakeAugurRepo()
	uc := NewAugurConversationUsecase(repo, nil)

	long := ""
	for i := 0; i < 200; i++ {
		long += "a"
	}
	conv, err := uc.EnsureConversation(context.Background(), uuid.New(), uuid.Nil, long)
	require.NoError(t, err)
	runes := []rune(conv.Title)
	assert.Equal(t, 81, len(runes), "expected 80 runes plus ellipsis")
	assert.Equal(t, '…', runes[len(runes)-1])
}

func TestEnsureConversation_ReturnsExistingWhenIDProvided(t *testing.T) {
	repo := newFakeAugurRepo()
	clock, _ := fixedClock(time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC))
	uc := NewAugurConversationUsecase(repo, clock)

	userID := uuid.New()
	first, err := uc.EnsureConversation(context.Background(), userID, uuid.Nil, "hello")
	require.NoError(t, err)

	again, err := uc.EnsureConversation(context.Background(), userID, first.ID, "different text")
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID)
	assert.Equal(t, first.Title, again.Title, "existing title must not be overwritten")
}

func TestEnsureConversation_RejectsOtherUsersID(t *testing.T) {
	repo := newFakeAugurRepo()
	uc := NewAugurConversationUsecase(repo, nil)

	ownerID := uuid.New()
	strangerID := uuid.New()
	_, err := uc.EnsureConversation(context.Background(), ownerID, uuid.Nil, "hello")
	require.NoError(t, err)

	// Another user passing a known id gets a *new* conversation, not access.
	conv, err := uc.EnsureConversation(context.Background(), strangerID, uuid.New(), "peek")
	require.NoError(t, err)
	assert.Equal(t, strangerID, conv.UserID)
}

func TestAppendTurns_PersistsInOrder(t *testing.T) {
	repo := newFakeAugurRepo()
	clock, _ := fixedClock(time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC))
	uc := NewAugurConversationUsecase(repo, clock)

	userID := uuid.New()
	conv, err := uc.EnsureConversation(context.Background(), userID, uuid.Nil, "first question")
	require.NoError(t, err)

	require.NoError(t, uc.AppendUserTurn(context.Background(), conv.ID, "first question"))
	require.NoError(t, uc.AppendAssistantTurn(context.Background(), conv.ID, "first answer", []domain.AugurCitation{{URL: "https://example.com", Title: "Ex"}}))
	require.NoError(t, uc.AppendUserTurn(context.Background(), conv.ID, "second question"))

	_, msgs, err := uc.GetConversation(context.Background(), userID, conv.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Equal(t, "first question", msgs[0].Content)
	assert.Equal(t, "assistant", msgs[1].Role)
	assert.Len(t, msgs[1].Citations, 1)
	assert.Equal(t, "user", msgs[2].Role)
}

func TestAppendTurns_RejectsEmptyContent(t *testing.T) {
	uc := NewAugurConversationUsecase(newFakeAugurRepo(), nil)
	err := uc.AppendUserTurn(context.Background(), uuid.New(), "   \n\t ")
	assert.Error(t, err)
}

func TestListConversations_SortsByLastActivity(t *testing.T) {
	repo := newFakeAugurRepo()
	clock, _ := fixedClock(time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC))
	uc := NewAugurConversationUsecase(repo, clock)

	userID := uuid.New()
	older, _ := uc.EnsureConversation(context.Background(), userID, uuid.Nil, "older")
	newer, _ := uc.EnsureConversation(context.Background(), userID, uuid.Nil, "newer")

	// append a late turn on `older` to bump its last activity
	require.NoError(t, uc.AppendUserTurn(context.Background(), older.ID, "ping"))

	summaries, err := uc.ListConversations(context.Background(), userID, 10, nil, nil)
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	assert.Equal(t, older.ID, summaries[0].ID, "bumped conversation must appear first")
	assert.Equal(t, newer.ID, summaries[1].ID)
	assert.Equal(t, 1, summaries[0].MessageCount)
	assert.Equal(t, "ping", summaries[0].LastMessagePreview)
}

func TestDeleteConversation_IsIdempotentAndScoped(t *testing.T) {
	repo := newFakeAugurRepo()
	uc := NewAugurConversationUsecase(repo, nil)

	userID := uuid.New()
	conv, err := uc.EnsureConversation(context.Background(), userID, uuid.Nil, "bye")
	require.NoError(t, err)
	require.NoError(t, uc.AppendUserTurn(context.Background(), conv.ID, "bye"))

	// delete twice: both succeed, second is a no-op
	require.NoError(t, uc.DeleteConversation(context.Background(), userID, conv.ID))
	require.NoError(t, uc.DeleteConversation(context.Background(), userID, conv.ID))

	got, msgs, err := uc.GetConversation(context.Background(), userID, conv.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Nil(t, msgs)
}
