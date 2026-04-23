package usecase

import (
	"context"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loopClock(t *testing.T) func() time.Time {
	t.Helper()
	base, err := time.Parse(time.RFC3339, "2026-04-23T10:00:00Z")
	require.NoError(t, err)
	return func() time.Time { return base }
}

func TestCreateSessionFromLoopEntry_RejectsNilUserID(t *testing.T) {
	uc := NewAugurConversationUsecase(newFakeAugurRepo(), loopClock(t))
	_, err := uc.CreateSessionFromLoopEntry(context.Background(), CreateSessionFromLoopEntryInput{
		UserID:   uuid.Nil,
		EntryKey: "entry-1",
		WhyText:  "a why",
	})
	require.Error(t, err)
}

func TestCreateSessionFromLoopEntry_RejectsEmptyEntryKey(t *testing.T) {
	uc := NewAugurConversationUsecase(newFakeAugurRepo(), loopClock(t))
	_, err := uc.CreateSessionFromLoopEntry(context.Background(), CreateSessionFromLoopEntryInput{
		UserID:  uuid.New(),
		WhyText: "a why",
	})
	require.Error(t, err)
}

func TestCreateSessionFromLoopEntry_RejectsEmptyWhyText(t *testing.T) {
	uc := NewAugurConversationUsecase(newFakeAugurRepo(), loopClock(t))
	_, err := uc.CreateSessionFromLoopEntry(context.Background(), CreateSessionFromLoopEntryInput{
		UserID:   uuid.New(),
		EntryKey: "entry-1",
		WhyText:  "   ",
	})
	require.Error(t, err)
}

func TestCreateSessionFromLoopEntry_CreatesConversationWithSeededAssistantTurn(t *testing.T) {
	repo := newFakeAugurRepo()
	uc := NewAugurConversationUsecase(repo, loopClock(t))
	userID := uuid.New()

	conv, err := uc.CreateSessionFromLoopEntry(context.Background(), CreateSessionFromLoopEntryInput{
		UserID:     userID,
		EntryKey:   "loop-entry-abc",
		LensModeID: "default",
		WhyText:    "Fresh long-form on OODA loops in knowledge work.",
		EvidenceRefs: []domain.AugurCitation{
			{URL: "https://example.com/ooda", Title: "primary source"},
			{URL: "https://example.com/boyd", Title: "Boyd primer"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, conv)
	assert.Equal(t, userID, conv.UserID)
	assert.NotEqual(t, uuid.Nil, conv.ID)
	assert.Contains(t, conv.Title, "OODA")

	msgs, err := repo.ListMessages(context.Background(), conv.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 1, "expected exactly one seeded turn")

	seed := msgs[0]
	assert.Equal(t, "assistant", seed.Role)
	assert.Contains(t, seed.Content, "OODA loops")
	require.Len(t, seed.Citations, 2)
	assert.Equal(t, "https://example.com/ooda", seed.Citations[0].URL)
	assert.Equal(t, "primary source", seed.Citations[0].Title)
}

func TestCreateSessionFromLoopEntry_TitleIsTruncatedTo80Runes(t *testing.T) {
	repo := newFakeAugurRepo()
	uc := NewAugurConversationUsecase(repo, loopClock(t))

	long := ""
	for range 300 {
		long += "x"
	}
	conv, err := uc.CreateSessionFromLoopEntry(context.Background(), CreateSessionFromLoopEntryInput{
		UserID:   uuid.New(),
		EntryKey: "k",
		WhyText:  long,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len([]rune(conv.Title)), 81) // 80 + ellipsis
}
