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
	gotGetArticleTagsReq        *datahubv1.GetArticleTagsRequest
	gotListArticlesByTagIDReq   *datahubv1.ListArticlesByTagIDRequest
	gotListArticlesByTagNameReq *datahubv1.ListArticlesByTagNameRequest
	gotFetchTagCloudReq         *datahubv1.FetchTagCloudRequest
	gotSearchByPrefixReq        *datahubv1.SearchTagsByPrefixRequest
	tags                        []*datahubv1.FeedTag
	trailArticles               []*datahubv1.TagTrailArticle
}

func (f *fakeTagDataHubClient) GetArticleTags(_ context.Context, req *connect.Request[datahubv1.GetArticleTagsRequest]) (*connect.Response[datahubv1.GetArticleTagsResponse], error) {
	f.gotGetArticleTagsReq = req.Msg
	return connect.NewResponse(&datahubv1.GetArticleTagsResponse{Tags: f.tags}), nil
}

func (f *fakeTagDataHubClient) ListArticlesByTagID(_ context.Context, req *connect.Request[datahubv1.ListArticlesByTagIDRequest]) (*connect.Response[datahubv1.ListArticlesByTagIDResponse], error) {
	f.gotListArticlesByTagIDReq = req.Msg
	return connect.NewResponse(&datahubv1.ListArticlesByTagIDResponse{Articles: f.trailArticles}), nil
}

func (f *fakeTagDataHubClient) ListArticlesByTagName(_ context.Context, req *connect.Request[datahubv1.ListArticlesByTagNameRequest]) (*connect.Response[datahubv1.ListArticlesByTagNameResponse], error) {
	f.gotListArticlesByTagNameReq = req.Msg
	return connect.NewResponse(&datahubv1.ListArticlesByTagNameResponse{Articles: f.trailArticles}), nil
}

func (f *fakeTagDataHubClient) FetchTagCloud(_ context.Context, req *connect.Request[datahubv1.FetchTagCloudRequest]) (*connect.Response[datahubv1.FetchTagCloudResponse], error) {
	f.gotFetchTagCloudReq = req.Msg
	return connect.NewResponse(&datahubv1.FetchTagCloudResponse{}), nil
}

func (f *fakeTagDataHubClient) SearchTagsByPrefix(_ context.Context, req *connect.Request[datahubv1.SearchTagsByPrefixRequest]) (*connect.Response[datahubv1.SearchTagsByPrefixResponse], error) {
	f.gotSearchByPrefixReq = req.Msg
	return connect.NewResponse(&datahubv1.SearchTagsByPrefixResponse{}), nil
}

func TestFetchTagCloud_StaysInstanceWide(t *testing.T) {
	client := &fakeTagDataHubClient{}
	gw := &TagGateway{client: client}

	_, err := gw.FetchTagCloud(context.Background(), 300)
	require.NoError(t, err)
	assert.Nil(t, client.gotFetchTagCloudReq.UserId, "the shared tag cloud must not carry a user scope")
}

func TestSearchTagsByPrefix_RejectsEmptyUserID(t *testing.T) {
	client := &fakeTagDataHubClient{}
	gw := &TagGateway{client: client}

	_, err := gw.SearchTagsByPrefix(context.Background(), "ai", "", 10)
	require.Error(t, err)
	assert.Nil(t, client.gotSearchByPrefixReq, "no RPC may be sent without a user id")
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

func TestFetchArticlesByTag_SendsUserIDFromContext(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	tagID := "22222222-2222-4222-8222-222222222222"

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	client := &fakeTagDataHubClient{
		trailArticles: []*datahubv1.TagTrailArticle{
			{Id: "a1", Title: "Tagged article"},
		},
	}
	gw := &TagGateway{client: client}

	articles, err := gw.FetchArticlesByTag(ctx, tagID, nil, 20)
	require.NoError(t, err)
	require.Len(t, articles, 1)
	require.NotNil(t, client.gotListArticlesByTagIDReq)
	assert.Equal(t, tagID, client.gotListArticlesByTagIDReq.GetTagId())
	assert.Equal(t, userID.String(), client.gotListArticlesByTagIDReq.GetUserId())
}

func TestFetchArticlesByTag_RequiresAuthenticatedUser(t *testing.T) {
	client := &fakeTagDataHubClient{}
	gw := &TagGateway{client: client}

	_, err := gw.FetchArticlesByTag(context.Background(), "22222222-2222-4222-8222-222222222222", nil, 20)
	require.Error(t, err)
	assert.Nil(t, client.gotListArticlesByTagIDReq)
}

func TestFetchArticlesByTagName_SendsUserIDFromContext(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	client := &fakeTagDataHubClient{
		trailArticles: []*datahubv1.TagTrailArticle{
			{Id: "a1", Title: "Cross-feed article"},
		},
	}
	gw := &TagGateway{client: client}

	articles, err := gw.FetchArticlesByTagName(ctx, "golang", nil, 20)
	require.NoError(t, err)
	require.Len(t, articles, 1)
	require.NotNil(t, client.gotListArticlesByTagNameReq)
	assert.Equal(t, "golang", client.gotListArticlesByTagNameReq.GetTagName())
	assert.Equal(t, userID.String(), client.gotListArticlesByTagNameReq.GetUserId())
}

func TestFetchArticlesByTagName_RequiresAuthenticatedUser(t *testing.T) {
	client := &fakeTagDataHubClient{}
	gw := &TagGateway{client: client}

	_, err := gw.FetchArticlesByTagName(context.Background(), "golang", nil, 20)
	require.Error(t, err)
	assert.Nil(t, client.gotListArticlesByTagNameReq)
}
