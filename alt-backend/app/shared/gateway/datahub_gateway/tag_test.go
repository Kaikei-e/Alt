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

type fakeTagDataHubClient struct {
	tagDataHubClient
	gotGetArticleTagsReq *datahubv1.GetArticleTagsRequest
	tags                 []*datahubv1.FeedTag
}

func (f *fakeTagDataHubClient) GetArticleTags(_ context.Context, req *connect.Request[datahubv1.GetArticleTagsRequest]) (*connect.Response[datahubv1.GetArticleTagsResponse], error) {
	f.gotGetArticleTagsReq = req.Msg
	return connect.NewResponse(&datahubv1.GetArticleTagsResponse{Tags: f.tags}), nil
}

func TestFetchArticleTags_SendsUserIDFromContext(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := "22222222-2222-4222-8222-222222222222"

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	client := &fakeTagDataHubClient{
		tags: []*datahubv1.FeedTag{
			{Id: "t1", TagName: "golang"},
		},
	}
	gw := &TagGateway{client: client}

	tags, err := gw.FetchArticleTags(ctx, articleID)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "golang", tags[0].TagName)
	require.NotNil(t, client.gotGetArticleTagsReq)
	assert.Equal(t, articleID, client.gotGetArticleTagsReq.GetArticleId())
	assert.Equal(t, userID.String(), client.gotGetArticleTagsReq.GetUserId())
}

func TestFetchArticleTags_RequiresAuthenticatedUser(t *testing.T) {
	client := &fakeTagDataHubClient{}
	gw := &TagGateway{client: client}

	_, err := gw.FetchArticleTags(context.Background(), "22222222-2222-4222-8222-222222222222")
	require.Error(t, err)
	assert.Nil(t, client.gotGetArticleTagsReq)
}
