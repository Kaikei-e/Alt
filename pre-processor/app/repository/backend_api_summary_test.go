package repository

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/domain"
	backend_api "pre-processor/driver/backend_api"
	datahubv1 "pre-processor/gen/proto/services/datahub/v1"
	"pre-processor/gen/proto/services/datahub/v1/datahubv1connect"
)

type mockSummaryDataHubClient struct {
	datahubv1connect.UnimplementedDataHubServiceHandler
	saveArticleSummaryFunc func(ctx context.Context, req *connect.Request[datahubv1.SaveArticleSummaryRequest]) (*connect.Response[datahubv1.SaveArticleSummaryResponse], error)
}

func (m *mockSummaryDataHubClient) SaveArticleSummary(ctx context.Context, req *connect.Request[datahubv1.SaveArticleSummaryRequest]) (*connect.Response[datahubv1.SaveArticleSummaryResponse], error) {
	return m.saveArticleSummaryFunc(ctx, req)
}

func TestSummaryRepository_Create_SummaryVersioning(t *testing.T) {
	t.Run("sets SUMMARY_VERSIONING_SKIP when SkipVersioning is true", func(t *testing.T) {
		var captured *datahubv1.SaveArticleSummaryRequest
		mock := &mockSummaryDataHubClient{
			saveArticleSummaryFunc: func(_ context.Context, req *connect.Request[datahubv1.SaveArticleSummaryRequest]) (*connect.Response[datahubv1.SaveArticleSummaryResponse], error) {
				captured = req.Msg
				return connect.NewResponse(&datahubv1.SaveArticleSummaryResponse{Success: true}), nil
			},
		}
		repo := NewSummaryRepository(backend_api.NewClientWithService(mock))

		err := repo.Create(context.Background(), &domain.ArticleSummary{
			ArticleID:       "art-001",
			UserID:          "user-001",
			ArticleTitle:    "Title",
			SummaryJapanese: domain.PlaceholderDeadLetter,
			SkipVersioning:  true,
		})
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Equal(t, datahubv1.SummaryVersioning_SUMMARY_VERSIONING_SKIP, captured.GetSummaryVersioning())
	})

	t.Run("leaves SummaryVersioning unset when SkipVersioning is false", func(t *testing.T) {
		var captured *datahubv1.SaveArticleSummaryRequest
		mock := &mockSummaryDataHubClient{
			saveArticleSummaryFunc: func(_ context.Context, req *connect.Request[datahubv1.SaveArticleSummaryRequest]) (*connect.Response[datahubv1.SaveArticleSummaryResponse], error) {
				captured = req.Msg
				return connect.NewResponse(&datahubv1.SaveArticleSummaryResponse{Success: true}), nil
			},
		}
		repo := NewSummaryRepository(backend_api.NewClientWithService(mock))

		err := repo.Create(context.Background(), &domain.ArticleSummary{
			ArticleID:       "art-001",
			UserID:          "user-001",
			ArticleTitle:    "Title",
			SummaryJapanese: domain.PlaceholderTooShort,
			SkipVersioning:  false,
		})
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Equal(t, datahubv1.SummaryVersioning_SUMMARY_VERSIONING_UNSPECIFIED, captured.GetSummaryVersioning())
	})
}
