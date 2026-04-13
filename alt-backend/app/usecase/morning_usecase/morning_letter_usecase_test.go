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

func TestGetLetterEnrichment_BuildsCardsWithAltHrefAndAcolyteLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)
	articleBatch := mocks.NewMockArticleMetadataBatchPort(ctrl)
	feedTitleBatch := mocks.NewMockFeedTitleBatchPort(ctrl)
	searchRelated := mocks.NewMockSearchRelatedArticlesPort(ctrl)

	articleID := uuid.New()
	feedID := uuid.New()

	// Sources are fetched (already subscription-filtered upstream in
	// the real GetLetterSources; here we let user feeds match).
	feedPort.EXPECT().GetUserFeedIDs(gomock.Any()).Return([]uuid.UUID{feedID}, nil)
	repo.EXPECT().GetLetterSources(gomock.Any(), "L1").Return(
		[]*domain.MorningLetterSourceEntry{
			{
				LetterID:   "L1",
				SectionKey: "top3",
				ArticleID:  articleID,
				SourceType: "overnight_group",
				Position:   0,
				FeedID:     feedID,
			},
		}, nil,
	)

	articleBatch.EXPECT().
		FetchArticlesByIDs(gomock.Any(), []uuid.UUID{articleID}).
		Return([]*domain.Article{
			{
				ID:      articleID,
				FeedID:  feedID,
				Title:   "SSL/TLS Certificates Explained",
				URL:     "https://example.com/tls",
				Content: "A long content body about TLS that should be trimmed into an excerpt for the morning letter card.",
				Tags:    []string{"security", "tls"},
			},
		}, nil)

	feedTitleBatch.EXPECT().
		FetchFeedTitlesByIDs(gomock.Any(), []uuid.UUID{feedID}).
		Return(map[uuid.UUID]string{feedID: "Example Weekly"}, nil)

	// Related returns one hit for the seed title.
	searchRelated.EXPECT().
		SearchArticles(gomock.Any(), "SSL/TLS Certificates Explained", "user-1").
		Return([]domain.SearchIndexerArticleHit{
			{ID: "rel-1", Title: "Deep dive into cert pinning"},
		}, nil)

	uc := NewMorningLetterUsecaseWithEnrichment(
		repo, feedPort, articleBatch, feedTitleBatch, searchRelated,
	)
	got, err := uc.GetLetterEnrichment(context.Background(), "L1", "user-1")

	require.NoError(t, err)
	require.Len(t, got, 1)
	e := got[0]
	assert.Equal(t, articleID.String(), e.ArticleID)
	assert.Equal(t, "top3", e.SectionKey)
	assert.Equal(t, "SSL/TLS Certificates Explained", e.ArticleTitle)
	assert.Equal(t, "https://example.com/tls", e.ArticleURL)
	assert.Equal(t, "/articles/"+articleID.String(), e.ArticleAltHref)
	assert.Equal(t, "Example Weekly", e.FeedTitle)
	assert.ElementsMatch(t, []string{"security", "tls"}, e.Tags)
	assert.Contains(t, e.AcolyteHref, "/acolyte/new?")
	assert.Contains(t, e.AcolyteHref, "article_id="+articleID.String())
	assert.Contains(t, e.AcolyteHref, "topic=")
	assert.Len(t, e.RelatedArticles, 1)
	assert.Equal(t, "Deep dive into cert pinning", e.RelatedArticles[0].Title)
	assert.Equal(t, "/articles/rel-1", e.RelatedArticles[0].ArticleAltHref)
	assert.NotEmpty(t, e.SummaryExcerpt)
}

func TestGetLetterEnrichment_GracefullyHandlesMissingArticle(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)
	articleBatch := mocks.NewMockArticleMetadataBatchPort(ctrl)
	feedTitleBatch := mocks.NewMockFeedTitleBatchPort(ctrl)
	searchRelated := mocks.NewMockSearchRelatedArticlesPort(ctrl)

	articleID := uuid.New()
	feedID := uuid.New()

	feedPort.EXPECT().GetUserFeedIDs(gomock.Any()).Return([]uuid.UUID{feedID}, nil)
	repo.EXPECT().GetLetterSources(gomock.Any(), "L2").Return(
		[]*domain.MorningLetterSourceEntry{
			{LetterID: "L2", SectionKey: "top3", ArticleID: articleID, FeedID: feedID},
		}, nil,
	)
	articleBatch.EXPECT().
		FetchArticlesByIDs(gomock.Any(), []uuid.UUID{articleID}).
		Return([]*domain.Article{}, nil) // article was deleted between letter and render
	feedTitleBatch.EXPECT().
		FetchFeedTitlesByIDs(gomock.Any(), gomock.Any()).
		Return(map[uuid.UUID]string{}, nil).AnyTimes()

	uc := NewMorningLetterUsecaseWithEnrichment(
		repo, feedPort, articleBatch, feedTitleBatch, searchRelated,
	)
	got, err := uc.GetLetterEnrichment(context.Background(), "L2", "user-1")

	require.NoError(t, err)
	require.Len(t, got, 1)
	// Even without article metadata, the card still has an alt_href so the
	// reader can click through to the stub article page.
	assert.Equal(t, "/articles/"+articleID.String(), got[0].ArticleAltHref)
	assert.Empty(t, got[0].ArticleTitle)
	assert.Empty(t, got[0].AcolyteHref)
}

func TestRegenerateLatest_FreshRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	doc := &domain.MorningLetterDocument{ID: "new-id", TargetDate: "2026-04-13"}
	repo.EXPECT().RegenerateLatest(gomock.Any(), "Asia/Tokyo").Return(doc, nil)

	uc := NewMorningLetterUsecase(repo, feedPort)
	got, regenerated, retryAfter, err := uc.RegenerateLatest(context.Background(), "user-1", "Asia/Tokyo")

	require.NoError(t, err)
	assert.True(t, regenerated)
	assert.Equal(t, "new-id", got.ID)
	assert.Equal(t, time.Duration(0), retryAfter)
}

func TestRegenerateLatest_RateLimited(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	doc := &domain.MorningLetterDocument{ID: "new-id"}
	// First call regenerates, second call should serve cached without calling RegenerateLatest.
	repo.EXPECT().RegenerateLatest(gomock.Any(), gomock.Any()).Return(doc, nil).Times(1)
	repo.EXPECT().GetLatestLetter(gomock.Any()).Return(doc, nil).Times(1)

	uc := NewMorningLetterUsecase(repo, feedPort)
	_, _, _, err := uc.RegenerateLatest(context.Background(), "user-1", "Asia/Tokyo")
	require.NoError(t, err)

	got, regenerated, retryAfter, err := uc.RegenerateLatest(context.Background(), "user-1", "Asia/Tokyo")
	require.NoError(t, err)
	assert.False(t, regenerated)
	assert.Equal(t, "new-id", got.ID)
	assert.Greater(t, retryAfter, time.Duration(0))
}

func TestRegenerateLatest_FailureClearsCooldown(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMorningLetterRepository(ctrl)
	feedPort := mocks.NewMockUserFeedPort(ctrl)

	doc := &domain.MorningLetterDocument{ID: "new-id"}
	// Error on first attempt should not arm the cooldown; second attempt regenerates.
	repo.EXPECT().RegenerateLatest(gomock.Any(), gomock.Any()).Return(nil, assert.AnError).Times(1)
	repo.EXPECT().RegenerateLatest(gomock.Any(), gomock.Any()).Return(doc, nil).Times(1)

	uc := NewMorningLetterUsecase(repo, feedPort)
	_, _, _, err := uc.RegenerateLatest(context.Background(), "user-1", "Asia/Tokyo")
	require.Error(t, err)

	got, regenerated, _, err := uc.RegenerateLatest(context.Background(), "user-1", "Asia/Tokyo")
	require.NoError(t, err)
	assert.True(t, regenerated)
	assert.Equal(t, "new-id", got.ID)
}
