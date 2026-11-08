package datahub_gateway

import (
	"context"
	"testing"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOgImageDataHubClient struct {
	ogImageDataHubClient
	gotBatchGetReq *datahubv1.BatchGetOgImageURLsRequest
	urls           map[string]string
}

func (f *fakeOgImageDataHubClient) BatchGetOgImageURLs(_ context.Context, req *connect.Request[datahubv1.BatchGetOgImageURLsRequest]) (*connect.Response[datahubv1.BatchGetOgImageURLsResponse], error) {
	f.gotBatchGetReq = req.Msg
	return connect.NewResponse(&datahubv1.BatchGetOgImageURLsResponse{OgImageUrls: f.urls}), nil
}

func TestFetchOgImageURLsByArticleIDs_SendsUserIDFromContext(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := "22222222-2222-4222-8222-222222222222"

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	client := &fakeOgImageDataHubClient{
		urls: map[string]string{articleID: "https://cdn.example.com/og.png"},
	}
	gw := &OgImageGateway{client: client}

	urls, err := gw.FetchOgImageURLsByArticleIDs(ctx, []string{articleID})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{articleID: "https://cdn.example.com/og.png"}, urls)
	require.NotNil(t, client.gotBatchGetReq)
	assert.Equal(t, []string{articleID}, client.gotBatchGetReq.GetArticleIds())
	assert.Equal(t, userID.String(), client.gotBatchGetReq.GetUserId())
}

func TestFetchOgImageURLsByArticleIDs_RequiresAuthenticatedUser(t *testing.T) {
	client := &fakeOgImageDataHubClient{}
	gw := &OgImageGateway{client: client}

	_, err := gw.FetchOgImageURLsByArticleIDs(context.Background(), []string{"22222222-2222-4222-8222-222222222222"})
	require.Error(t, err)
	assert.Nil(t, client.gotBatchGetReq)
}
