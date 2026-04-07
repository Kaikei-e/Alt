package morning_gateway

import (
	"context"
	"encoding/json"
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
	// All dropped because altDBRepository is nil → no feed_id lookup possible
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
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		recapWorkerURL: serverURL,
	}
}

func strPtr(s string) *string {
	return &s
}
