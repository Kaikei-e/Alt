package recall_rail_usecase

import (
	"alt/domain"
	"alt/port/recall_candidate_port"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

type mockCandidatePort struct {
	candidates []domain.RecallCandidate
	err        error
}

var _ recall_candidate_port.GetRecallCandidatesPort = (*mockCandidatePort)(nil)

func (m *mockCandidatePort) GetRecallCandidates(_ context.Context, _ uuid.UUID, _ int) ([]domain.RecallCandidate, error) {
	return m.candidates, m.err
}

type mockFeatureFlag struct {
	enabled bool
}

func (m *mockFeatureFlag) IsEnabled(_ string, _ uuid.UUID) bool { return m.enabled }

type mockFallbackPort struct {
	articles map[string]fallbackArticle
	calls    []string
}

type fallbackArticle struct {
	title       string
	link        string
	publishedAt *time.Time
}

var _ recall_candidate_port.ArticleFallbackPort = (*mockFallbackPort)(nil)

func (m *mockFallbackPort) GetArticleTitleAndLink(_ context.Context, articleID string) (string, string, *time.Time, error) {
	m.calls = append(m.calls, articleID)
	a, ok := m.articles[articleID]
	if !ok {
		return "", "", nil, nil
	}
	return a.title, a.link, a.publishedAt, nil
}

// --- tests ---

func TestExecute_FallbackEnrichesNilItems(t *testing.T) {
	articleID := uuid.New()
	now := time.Now().Truncate(time.Second)

	candidatePort := &mockCandidatePort{
		candidates: []domain.RecallCandidate{
			{
				UserID:  uuid.New(),
				ItemKey: "article:" + articleID.String(),
				Item:    nil, // missing KHI
			},
		},
	}
	fallback := &mockFallbackPort{
		articles: map[string]fallbackArticle{
			articleID.String(): {title: "Test Article", link: "https://example.com/article", publishedAt: &now},
		},
	}

	uc := NewRecallRailUsecase(candidatePort, &mockFeatureFlag{enabled: true}, fallback)
	candidates, err := uc.Execute(context.Background(), uuid.New(), 5)
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	item := candidates[0].Item
	require.NotNil(t, item, "fallback should have enriched the nil item")
	assert.Equal(t, "Test Article", item.Title)
	assert.Equal(t, "https://example.com/article", item.Link)
	assert.Equal(t, &now, item.PublishedAt)
	assert.Equal(t, domain.ItemArticle, item.ItemType)
	assert.Equal(t, domain.SummaryStateMissing, item.SummaryState)
	assert.Equal(t, &articleID, item.PrimaryRefID)
	assert.Equal(t, "article:"+articleID.String(), item.ItemKey)
}

func TestExecute_FallbackSkipsNonArticleKeys(t *testing.T) {
	candidatePort := &mockCandidatePort{
		candidates: []domain.RecallCandidate{
			{ItemKey: "recap_anchor:" + uuid.New().String(), Item: nil},
			{ItemKey: "pulse_anchor:" + uuid.New().String(), Item: nil},
		},
	}
	fallback := &mockFallbackPort{articles: map[string]fallbackArticle{}}

	uc := NewRecallRailUsecase(candidatePort, &mockFeatureFlag{enabled: true}, fallback)
	candidates, err := uc.Execute(context.Background(), uuid.New(), 5)
	require.NoError(t, err)
	require.Len(t, candidates, 2)

	assert.Nil(t, candidates[0].Item)
	assert.Nil(t, candidates[1].Item)
	assert.Empty(t, fallback.calls, "should not call fallback for non-article keys")
}

func TestExecute_FallbackGracefulOnArticleNotFound(t *testing.T) {
	candidatePort := &mockCandidatePort{
		candidates: []domain.RecallCandidate{
			{ItemKey: "article:" + uuid.New().String(), Item: nil},
		},
	}
	fallback := &mockFallbackPort{articles: map[string]fallbackArticle{}} // empty — article not found

	uc := NewRecallRailUsecase(candidatePort, &mockFeatureFlag{enabled: true}, fallback)
	candidates, err := uc.Execute(context.Background(), uuid.New(), 5)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Nil(t, candidates[0].Item, "should remain nil when article not found")
}

func TestExecute_FallbackPortNil(t *testing.T) {
	articleID := uuid.New()
	candidatePort := &mockCandidatePort{
		candidates: []domain.RecallCandidate{
			{ItemKey: "article:" + articleID.String(), Item: nil},
		},
	}

	uc := NewRecallRailUsecase(candidatePort, &mockFeatureFlag{enabled: true}, nil)
	candidates, err := uc.Execute(context.Background(), uuid.New(), 5)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Nil(t, candidates[0].Item, "should remain nil when fallback port is nil")
}

func TestExecute_NoFallbackWhenItemPresent(t *testing.T) {
	articleID := uuid.New()
	existingItem := &domain.KnowledgeHomeItem{
		ItemKey: "article:" + articleID.String(),
		Title:   "Already Enriched",
	}
	candidatePort := &mockCandidatePort{
		candidates: []domain.RecallCandidate{
			{ItemKey: "article:" + articleID.String(), Item: existingItem},
		},
	}
	fallback := &mockFallbackPort{articles: map[string]fallbackArticle{}}

	uc := NewRecallRailUsecase(candidatePort, &mockFeatureFlag{enabled: true}, fallback)
	candidates, err := uc.Execute(context.Background(), uuid.New(), 5)
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	assert.Equal(t, "Already Enriched", candidates[0].Item.Title)
	assert.Empty(t, fallback.calls, "should not call fallback when item is already present")
}
