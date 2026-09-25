package handler

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestValidateAndBuildLensFilter(t *testing.T) {
	tests := []struct {
		name string
		in   *sovereignv1.LensFilter
		want *sovereign_db.LensFilter
	}{
		{
			name: "nil input returns nil",
			in:   nil,
			want: nil,
		},
		{
			name: "populated filter maps correctly",
			in: &sovereignv1.LensFilter{
				TagIds:       []string{"go", "solid"},
				SourceIds:    []string{"source-1", "source-2"},
				TimeWindow:   "30d",
				QueryText:    "clean architecture",
				IncludeRecap: true,
				IncludePulse: false,
				SortMode:     "date_desc",
			},
			want: &sovereign_db.LensFilter{
				QueryText:    "clean architecture",
				TagNames:     []string{"go", "solid"},
				SourceIDs:    []string{"source-1", "source-2"},
				TimeWindow:   "30d",
				IncludeRecap: true,
				IncludePulse: false,
				SortMode:     "date_desc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateAndBuildLensFilter(tt.in)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestLensMappers(t *testing.T) {
	lensID := uuid.New()
	userID := uuid.New()
	tenantID := uuid.New()
	versionID := uuid.New()
	created := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	archived := time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC)

	version := sovereign_db.KnowledgeLensVersion{
		LensVersionID: versionID,
		LensID:        lensID,
		CreatedAt:     created,
		QueryText:     "search query",
		TagIDs:        []string{"tag1"},
		SourceIDs:     []string{"source1"},
		TimeWindow:    "7d",
		IncludeRecap:  true,
		IncludePulse:  true,
		SortMode:      "relevance",
	}

	lens := sovereign_db.KnowledgeLens{
		LensID:         lensID,
		UserID:         userID,
		TenantID:       tenantID,
		Name:           "Tech Lens",
		Description:    "Tech articles",
		CreatedAt:      created,
		UpdatedAt:      created,
		ArchivedAt:     &archived,
		CurrentVersion: &version,
	}

	pbLens := lensToProto(lens)
	require.NotNil(t, pbLens)
	assert.Equal(t, lensID.String(), pbLens.LensId)
	assert.Equal(t, "Tech Lens", pbLens.Name)
	assert.True(t, pbLens.ArchivedAt.AsTime().Equal(archived))
	require.NotNil(t, pbLens.CurrentVersion)
	assert.Equal(t, versionID.String(), pbLens.CurrentVersion.LensVersionId)
	assert.Equal(t, "search query", pbLens.CurrentVersion.QueryText)
}

func TestGetLens_PopulatesCurrentVersion(t *testing.T) {
	lensID := uuid.New()
	versionID := uuid.New()

	tests := []struct {
		name        string
		repo        *mockRepo
		wantVersion bool
		wantNilLens bool
	}{
		{
			name: "lens with current version",
			repo: &mockRepo{
				returnLens: &sovereign_db.KnowledgeLens{
					LensID: lensID,
					UserID: uuid.New(),
					Name:   "Test Lens",
				},
				returnVersion: &sovereign_db.KnowledgeLensVersion{
					LensVersionID: versionID,
					LensID:        lensID,
					TagIDs:        []string{"llm", "ai"},
					TimeWindow:    "90d",
					SortMode:      "relevance",
				},
			},
			wantVersion: true,
		},
		{
			name: "lens without version",
			repo: &mockRepo{
				returnLens: &sovereign_db.KnowledgeLens{
					LensID: lensID,
					UserID: uuid.New(),
					Name:   "Orphan Lens",
				},
				returnVersion: nil,
			},
			wantVersion: false,
		},
		{
			name:        "lens not found",
			repo:        &mockRepo{},
			wantNilLens: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, cleanup := setupTestServer(tt.repo)
			defer cleanup()

			resp, err := client.GetLens(context.Background(),
				connect.NewRequest(&sovereignv1.GetLensRequest{
					LensId: lensID.String(),
				}))

			require.NoError(t, err)

			if tt.wantNilLens {
				assert.Nil(t, resp.Msg.Lens)
				return
			}

			require.NotNil(t, resp.Msg.Lens)

			if tt.wantVersion {
				require.NotNil(t, resp.Msg.Lens.CurrentVersion, "CurrentVersion should be populated")
				assert.Equal(t, versionID.String(), resp.Msg.Lens.CurrentVersion.LensVersionId)
				assert.Equal(t, []string{"llm", "ai"}, resp.Msg.Lens.CurrentVersion.TagIds)
				assert.Equal(t, "90d", resp.Msg.Lens.CurrentVersion.TimeWindow)
			} else {
				assert.Nil(t, resp.Msg.Lens.CurrentVersion)
			}
		})
	}
}

func TestAreArticlesVisibleInLens_ValidatesRequiredFields(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	// Missing tenant_id & user_id
	_, err := h.AreArticlesVisibleInLens(context.Background(),
		connect.NewRequest(&sovereignv1.AreArticlesVisibleInLensRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	// Invalid article_id
	_, err = h.AreArticlesVisibleInLens(context.Background(),
		connect.NewRequest(&sovereignv1.AreArticlesVisibleInLensRequest{
			TenantId:   uuid.New().String(),
			UserId:     uuid.New().String(),
			ArticleIds: []string{"invalid-uuid"},
		}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
