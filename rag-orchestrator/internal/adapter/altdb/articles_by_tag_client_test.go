package altdb

import (
	"context"
	"testing"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"rag-orchestrator/internal/domain"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubArticlesByTagDataHubClient struct {
	datahubv1connect.DataHubServiceClient

	gotRequest *datahubv1.FetchArticlesByTagRequest
	response   *datahubv1.FetchArticlesByTagResponse
}

func (s *stubArticlesByTagDataHubClient) FetchArticlesByTag(
	_ context.Context,
	req *connect.Request[datahubv1.FetchArticlesByTagRequest],
) (*connect.Response[datahubv1.FetchArticlesByTagResponse], error) {
	s.gotRequest = req.Msg
	return connect.NewResponse(s.response), nil
}

func TestDataHubArticlesByTagClient_SendsUserIDFromContext(t *testing.T) {
	stub := &stubArticlesByTagDataHubClient{
		response: &datahubv1.FetchArticlesByTagResponse{
			Articles: []*datahubv1.ArticleByTagItem{
				{Id: "a1", Title: "Tagged", Url: "https://example.com/a1"},
			},
		},
	}
	client := NewDataHubArticlesByTagClient(stub, testLogger())

	ctx := domain.WithUserID(context.Background(), "11111111-1111-4111-8111-111111111111")
	articles, err := client.FetchArticlesByTag(ctx, "golang", 10)
	require.NoError(t, err)
	require.Len(t, articles, 1)
	require.NotNil(t, stub.gotRequest)
	assert.Equal(t, "golang", stub.gotRequest.GetTagName())
	assert.Equal(t, int32(10), stub.gotRequest.GetLimit())
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", stub.gotRequest.GetUserId())
}

func TestDataHubArticlesByTagClient_RejectsMissingUserID(t *testing.T) {
	stub := &stubArticlesByTagDataHubClient{
		response: &datahubv1.FetchArticlesByTagResponse{},
	}
	client := NewDataHubArticlesByTagClient(stub, testLogger())

	_, err := client.FetchArticlesByTag(context.Background(), "golang", 10)
	require.Error(t, err)
	assert.Nil(t, stub.gotRequest)
}
