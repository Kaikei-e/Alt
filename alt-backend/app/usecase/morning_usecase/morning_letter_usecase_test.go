package morning_usecase

import (
	"context"
	"testing"
	"time"

	"alt/domain"
	"alt/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetLatestLetter_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	doc := &domain.MorningLetterDocument{
		ID:              "test-id",
		TargetDate:      "2026-04-07",
		EditionTimezone: "Asia/Tokyo",
		SchemaVersion:   1,
		Body: domain.MorningLetterBody{
			Lead: "Today's briefing",
		},
	}
	repo.EXPECT().GetLatestLetter(gomock.Any()).Return(doc, nil)

	uc := NewMorningLetterUsecase(repo, feedPort)
	result, err := uc.GetLatestLetter(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "test-id", result.ID)
	assert.Equal(t, "Today's briefing", result.Body.Lead)
}

func TestGetLatestLetter_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	repo.EXPECT().GetLatestLetter(gomock.Any()).Return(nil, nil)

	uc := NewMorningLetterUsecase(repo, feedPort)
	result, err := uc.GetLatestLetter(context.Background())

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGetLetterByDate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	doc := &domain.MorningLetterDocument{
		ID:         "date-id",
		TargetDate: "2026-04-07",
	}
	repo.EXPECT().GetLetterByDate(gomock.Any(), "2026-04-07").Return(doc, nil)

	uc := NewMorningLetterUsecase(repo, feedPort)
	result, err := uc.GetLetterByDate(context.Background(), "2026-04-07")

	require.NoError(t, err)
	assert.Equal(t, "date-id", result.ID)
}

func TestGetLetterByDate_InvalidDate(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	uc := NewMorningLetterUsecase(repo, feedPort)
	_, err := uc.GetLetterByDate(context.Background(), "not-a-date")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid date format")
}

func TestGetLetterSources_FiltersBySubscription(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	feed1 := uuid.New()
	feed2 := uuid.New()
	feedUnsubscribed := uuid.New()

	sources := []*domain.MorningLetterSourceEntry{
		{LetterID: "l1", SectionKey: "top3", ArticleID: uuid.New(), FeedID: feed1, Position: 0},
		{LetterID: "l1", SectionKey: "top3", ArticleID: uuid.New(), FeedID: feedUnsubscribed, Position: 1},
		{LetterID: "l1", SectionKey: "what_changed", ArticleID: uuid.New(), FeedID: feed2, Position: 0},
		{LetterID: "l1", SectionKey: "what_changed", ArticleID: uuid.New(), FeedID: feedUnsubscribed, Position: 1},
		{LetterID: "l1", SectionKey: "by_genre:tech", ArticleID: uuid.New(), FeedID: feed1, Position: 0},
	}

	repo.EXPECT().GetLetterSources(gomock.Any(), "l1").Return(sources, nil)
	feedPort.EXPECT().GetUserFeedIDs(gomock.Any()).Return([]uuid.UUID{feed1, feed2}, nil)

	uc := NewMorningLetterUsecase(repo, feedPort)
	result, err := uc.GetLetterSources(context.Background(), "l1")

	require.NoError(t, err)
	assert.Len(t, result, 3) // feed1 x2, feed2 x1 — feedUnsubscribed x2 dropped
}

func TestGetLetterSources_NoSubscribedSources(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	feedUnsubscribed := uuid.New()
	sources := []*domain.MorningLetterSourceEntry{
		{LetterID: "l1", SectionKey: "top3", ArticleID: uuid.New(), FeedID: feedUnsubscribed, Position: 0},
	}

	repo.EXPECT().GetLetterSources(gomock.Any(), "l1").Return(sources, nil)
	feedPort.EXPECT().GetUserFeedIDs(gomock.Any()).Return([]uuid.UUID{uuid.New()}, nil)

	uc := NewMorningLetterUsecase(repo, feedPort)
	result, err := uc.GetLetterSources(context.Background(), "l1")

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestGetLetterSources_AllSubscribed(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	feed1 := uuid.New()
	sources := []*domain.MorningLetterSourceEntry{
		{LetterID: "l1", SectionKey: "top3", ArticleID: uuid.New(), FeedID: feed1, Position: 0},
		{LetterID: "l1", SectionKey: "top3", ArticleID: uuid.New(), FeedID: feed1, Position: 1},
	}

	repo.EXPECT().GetLetterSources(gomock.Any(), "l1").Return(sources, nil)
	feedPort.EXPECT().GetUserFeedIDs(gomock.Any()).Return([]uuid.UUID{feed1}, nil)

	uc := NewMorningLetterUsecase(repo, feedPort)
	result, err := uc.GetLetterSources(context.Background(), "l1")

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// Suppress unused import warnings for time package (used in domain model)
var _ = time.Now
