package recap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

func TestDomainToProto_TableDriven(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	articleID := "art-1"

	tests := []struct {
		name     string
		input    *domain.RecapSummary
		expected int // expected genres count
	}{
		{
			name: "summary with genres and references",
			input: &domain.RecapSummary{
				JobID:         "job-7d",
				ExecutedAt:    now,
				WindowStart:   now.Add(-7 * 24 * time.Hour),
				WindowEnd:     now,
				TotalArticles: 100,
				Genres: []domain.RecapGenre{
					{
						Genre:        "Tech",
						Summary:      "Tech summary",
						TopTerms:     []string{"go", "grpc"},
						ArticleCount: 10,
						ClusterCount: 2,
						EvidenceLinks: []domain.EvidenceLink{
							{
								ArticleID:   "art-1",
								Title:       "Go 1.26",
								SourceURL:   "https://golang.org",
								PublishedAt: "2026-02-01",
								Lang:        "en",
							},
						},
						Bullets: []string{"bullet 1"},
						References: []domain.Reference{
							{
								ID:        1,
								URL:       "https://example.com/ref",
								Domain:    "example.com",
								ArticleID: &articleID,
							},
						},
					},
				},
				SurfaceStatus: &domain.RecapSurfaceStatus{
					GeneratedAt: "2026-02-01T12:00:00Z",
					IsStale:     false,
					StaleAfter:  "2026-02-02T18:00:00Z",
					LatestRun: &domain.RecapSurfaceRun{
						JobID:     "job-7d",
						Status:    "completed",
						KickedAt:  "2026-02-01T11:55:00Z",
						UpdatedAt: "2026-02-01T12:00:00Z",
					},
					DegradationReasons: []domain.RecapDegradationReason{
						{Code: "extractive_genres", Detail: "1 of 1 genres are extractive excerpts, not AI summaries"},
					},
				},
			},
			expected: 1,
		},
		{
			name: "empty summary",
			input: &domain.RecapSummary{
				JobID:       "job-empty",
				ExecutedAt:  now,
				WindowStart: now,
				WindowEnd:   now,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res7 := domainToProto(tt.input)
			require.NotNil(t, res7)
			assert.Equal(t, tt.input.JobID, res7.JobId)
			assert.Len(t, res7.Genres, tt.expected)

			res3 := domainToProtoThreeDays(tt.input)
			require.NotNil(t, res3)
			assert.Equal(t, tt.input.JobID, res3.JobId)
			assert.Len(t, res3.Genres, tt.expected)
			if tt.input.SurfaceStatus != nil {
				require.NotNil(t, res3.SurfaceStatus)
				assert.Equal(t, tt.input.SurfaceStatus.GeneratedAt, res3.SurfaceStatus.GeneratedAt)
				require.Len(t, res3.SurfaceStatus.DegradationReasons, 1)
				assert.Equal(t, "extractive_genres", res3.SurfaceStatus.DegradationReasons[0].Code)
			} else {
				assert.Nil(t, res3.SurfaceStatus)
			}
		})
	}
}

func TestDomainToProtoTopicCards_TableDriven(t *testing.T) {
	tests := []struct {
		name             string
		input            *domain.RecapCardsResponse
		expectedCards    int
		hasJob           bool
		hasSurfaceStatus bool
		runStatus        string
	}{
		{
			name:             "nil response returns empty cards and nil surface_status",
			input:            nil,
			expectedCards:    0,
			hasJob:           false,
			hasSurfaceStatus: false,
		},
		{
			name: "response with cards, sources, and completed surface_status",
			input: &domain.RecapCardsResponse{
				Job: &domain.RecapCardsJob{
					JobID:         "job-cards",
					CardsSelected: 2,
				},
				Cards: []*domain.RecapCard{
					{
						ID:         "card-1",
						Rank:       1,
						HeadlineJa: "Headline",
						Sources: []*domain.RecapCardSource{
							{
								N:     1,
								Title: "Source 1",
								URL:   "https://example.com",
							},
						},
					},
					nil,
				},
				SurfaceStatus: &domain.RecapSurfaceStatus{
					GeneratedAt: "2026-09-22T17:05:00Z",
					WindowStart: "2026-09-19T17:00:00Z",
					WindowEnd:   "2026-09-22T17:00:00Z",
					IsStale:     false,
					StaleAfter:  "2026-09-23T23:05:00Z",
					LatestRun: &domain.RecapSurfaceRun{
						JobID:     "job-cards",
						Status:    "completed",
						KickedAt:  "2026-09-22T17:00:00Z",
						UpdatedAt: "2026-09-22T17:05:00Z",
					},
					DegradationReasons: []domain.RecapDegradationReason{
						{Code: "few_cards", Detail: "2 cards selected"},
					},
				},
			},
			expectedCards:    1,
			hasJob:           true,
			hasSurfaceStatus: true,
			runStatus:        "completed",
		},
		{
			name: "response with failed latest_run",
			input: &domain.RecapCardsResponse{
				Job: &domain.RecapCardsJob{
					JobID: "job-completed",
				},
				Cards: []*domain.RecapCard{},
				SurfaceStatus: &domain.RecapSurfaceStatus{
					GeneratedAt: "2026-09-22T17:05:00Z",
					LatestRun: &domain.RecapSurfaceRun{
						JobID:     "job-failed",
						Status:    "failed",
						KickedAt:  "2026-09-22T18:00:00Z",
						UpdatedAt: "2026-09-22T18:02:00Z",
					},
				},
			},
			expectedCards:    0,
			hasJob:           true,
			hasSurfaceStatus: true,
			runStatus:        "failed",
		},
		{
			name: "response with nil surface_status",
			input: &domain.RecapCardsResponse{
				Job:           nil,
				Cards:         []*domain.RecapCard{},
				SurfaceStatus: nil,
			},
			expectedCards:    0,
			hasJob:           false,
			hasSurfaceStatus: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protoResp := domainToProtoTopicCards(tt.input)
			require.NotNil(t, protoResp)
			assert.Len(t, protoResp.Cards, tt.expectedCards)
			if tt.hasJob {
				assert.NotNil(t, protoResp.Job)
				assert.Equal(t, tt.input.Job.JobID, protoResp.Job.JobId)
			} else {
				assert.Nil(t, protoResp.Job)
			}
			if tt.hasSurfaceStatus {
				require.NotNil(t, protoResp.SurfaceStatus)
				assert.Equal(t, tt.input.SurfaceStatus.GeneratedAt, protoResp.SurfaceStatus.GeneratedAt)
				require.NotNil(t, protoResp.SurfaceStatus.LatestRun)
				assert.Equal(t, tt.input.SurfaceStatus.LatestRun.JobID, protoResp.SurfaceStatus.LatestRun.JobId)
				assert.Equal(t, tt.runStatus, protoResp.SurfaceStatus.LatestRun.Status)
				assert.Equal(t, tt.input.SurfaceStatus.LatestRun.KickedAt, protoResp.SurfaceStatus.LatestRun.KickedAt)
				assert.Equal(t, tt.input.SurfaceStatus.LatestRun.UpdatedAt, protoResp.SurfaceStatus.LatestRun.UpdatedAt)
				assert.Len(t, protoResp.SurfaceStatus.DegradationReasons, len(tt.input.SurfaceStatus.DegradationReasons))
			} else {
				assert.Nil(t, protoResp.SurfaceStatus)
			}
		})
	}
}

func TestClusterDraftToProto_TableDriven(t *testing.T) {
	now := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		input     *domain.ClusterDraft
		wantDraft string
	}{
		{
			name: "populated draft",
			input: &domain.ClusterDraft{
				ID:          "draft-1",
				Description: "test draft",
				GeneratedAt: now,
				Genres: []domain.ClusterGenre{
					{
						Genre:        "Tech",
						SampleSize:   50,
						ClusterCount: 1,
						Clusters: []domain.ClusterSegment{
							{
								ClusterID: "101",
								Label:     "AI",
								RepresentativeArticles: []domain.ClusterArticle{
									{
										ArticleID: "art-101",
										Strategy:  "embedding",
									},
								},
							},
						},
					},
				},
			},
			wantDraft: "draft-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := clusterDraftToProto(tt.input)
			require.NotNil(t, res)
			assert.Equal(t, tt.wantDraft, res.DraftId)
			assert.Len(t, res.Genres, 1)
			assert.Len(t, res.Genres[0].Clusters, 1)
			assert.Len(t, res.Genres[0].Clusters[0].RepresentativeArticles, 1)
		})
	}
}

func TestRecapSearchResultsToProto_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		input    []*domain.RecapSearchResult
		expected int
	}{
		{
			name:     "empty results",
			input:    nil,
			expected: 0,
		},
		{
			name: "single result",
			input: []*domain.RecapSearchResult{
				{
					JobID:      "job-sr",
					ExecutedAt: "2026-02-01T00:00:00Z",
					WindowDays: 7,
					Genre:      "Economy",
					Summary:    "Summary",
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protoItems := recapSearchResultsToProto(tt.input)
			assert.Len(t, protoItems, tt.expected)
			if tt.expected > 0 {
				assert.Equal(t, tt.input[0].JobID, protoItems[0].JobId)
				assert.Equal(t, tt.input[0].Genre, protoItems[0].Genre)
			}
		})
	}
}
