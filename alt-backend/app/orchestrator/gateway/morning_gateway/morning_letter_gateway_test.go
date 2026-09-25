package morning_gateway

import (
	"context"
	"encoding/json"

	"alt/domain"

	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLatestLetter_Success(t *testing.T) {
	body := MorningLetterAPIResponse{
		ID:                 uuid.New().String(),
		TargetDate:         "2026-04-07",
		EditionTimezone:    "Asia/Tokyo",
		IsDegraded:         false,
		SchemaVersion:      1,
		GenerationRevision: 1,
		Model:              strPtr("gemma4-e4b-12k"),
		CreatedAt:          time.Now().UTC().Format(time.RFC3339),
		Etag:               "\"test:1\"",
		Body: MorningLetterBodyAPI{
			Lead: "Today's top story",
			Sections: []MorningLetterSectionAPI{
				{Key: "top3", Title: "Top Stories", Bullets: []string{"Bullet 1"}},
			},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/morning/letters/latest", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	gw := newTestGateway(t, server.URL)
	result, err := gw.GetLatestLetter(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, body.ID, result.ID)
	assert.Equal(t, "Today's top story", result.Body.Lead)
	assert.Len(t, result.Body.Sections, 1)
}

func TestGetLatestLetter_RecapWorker404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	gw := newTestGateway(t, server.URL)
	result, err := gw.GetLatestLetter(context.Background())

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGetLatestLetter_RecapWorkerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	gw := newTestGateway(t, server.URL)
	_, err := gw.GetLatestLetter(context.Background())

	require.Error(t, err)
}

func TestGetLetterByDate_Success(t *testing.T) {
	body := MorningLetterAPIResponse{
		ID:         uuid.New().String(),
		TargetDate: "2026-04-07",
		Body: MorningLetterBodyAPI{
			Lead: "Date-specific letter",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/morning/letters/2026-04-07", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	gw := newTestGateway(t, server.URL)
	result, err := gw.GetLetterByDate(context.Background(), "2026-04-07")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Date-specific letter", result.Body.Lead)
}

func TestGetLetterSources_WithSources(t *testing.T) {
	articleID1 := uuid.New()
	articleID2 := uuid.New()

	apiSources := []MorningLetterSourceAPI{
		{LetterID: "l1", SectionKey: "top3", ArticleID: articleID1.String(), SourceType: "recap", Position: 0},
		{LetterID: "l1", SectionKey: "top3", ArticleID: articleID2.String(), SourceType: "overnight", Position: 1},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/morning/letters/")
		assert.Contains(t, r.URL.Path, "/sources")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiSources)
	}))
	defer server.Close()

	// Without DB repository, sources with unknown feed_id are dropped (with warn log)
	gw := newTestGateway(t, server.URL)
	result, err := gw.GetLetterSources(context.Background(), "l1")

	require.NoError(t, err)
	// All dropped because the article reader resolves neither id → no feed_id
	assert.Empty(t, result)
}

func TestGetLetterSources_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	gw := newTestGateway(t, server.URL)
	result, err := gw.GetLetterSources(context.Background(), uuid.New().String())

	require.NoError(t, err)
	assert.Empty(t, result)
}

func newTestGateway(t *testing.T, serverURL string) *MorningLetterGateway {
	t.Helper()
	return &MorningLetterGateway{
		// An article reader that knows nothing: the sources it cannot resolve
		// a feed_id for are dropped, which is the behaviour under test.
		articles:       stubArticleBatchReader{},
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		recapWorkerURL: serverURL,
	}
}

// stubArticleBatchReader stands in for the alt-data-hub article batch read
// (catalog §2.C W3-C5) these gateways use to resolve feed ids.
type stubArticleBatchReader struct {
	articles []*domain.Article
}

func (s stubArticleBatchReader) FetchArticlesByIDs(_ context.Context, _ []uuid.UUID) ([]*domain.Article, error) {
	return s.articles, nil
}

func strPtr(s string) *string {
	return &s
}

func TestExtractSourceArticleIDs(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	sources := []MorningLetterSourceAPI{
		{ArticleID: id1.String()},
		{ArticleID: "invalid-uuid"},
		{ArticleID: id2.String()},
	}

	ids := extractSourceArticleIDs(sources)
	assert.Equal(t, []uuid.UUID{id1, id2}, ids)
}

func TestMapSourcesToDomain(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	feedID1 := uuid.New()
	sources := []MorningLetterSourceAPI{
		{LetterID: "let-1", SectionKey: "sec-1", ArticleID: id1.String(), SourceType: "article", Position: 1},
		{LetterID: "let-1", SectionKey: "sec-1", ArticleID: id2.String(), SourceType: "article", Position: 2},
		{LetterID: "let-1", SectionKey: "sec-1", ArticleID: "invalid", SourceType: "article", Position: 3},
	}
	feedMap := map[uuid.UUID]uuid.UUID{
		id1: feedID1,
	}

	mapped, dropped := mapSourcesToDomain(sources, feedMap)
	require.Len(t, mapped, 1)
	assert.Equal(t, id1, mapped[0].ArticleID)
	assert.Equal(t, feedID1, mapped[0].FeedID)
	assert.Equal(t, "let-1", mapped[0].LetterID)
	require.Len(t, dropped, 1)
	assert.Equal(t, id2.String(), dropped[0].ArticleID)
}

func TestBuildRegeneratePayload(t *testing.T) {
	b, err := buildRegeneratePayload("Asia/Tokyo")
	require.NoError(t, err)
	assert.Contains(t, string(b), `"edition_timezone":"Asia/Tokyo"`)
}
