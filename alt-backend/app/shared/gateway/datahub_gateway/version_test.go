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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeVersionDataHubClient struct {
	versionDataHubClient
	gotSupersedeReq *datahubv1.MarkSummaryVersionSupersededRequest
	gotByIDReq      *datahubv1.GetSummaryVersionByIDRequest
	gotLatestReq    *datahubv1.GetLatestSummaryVersionRequest
	prevVersion     *datahubv1.SummaryVersion
	byIDVersion     *datahubv1.SummaryVersion
	latestVersion   *datahubv1.SummaryVersion
}

func (f *fakeVersionDataHubClient) MarkSummaryVersionSuperseded(_ context.Context, req *connect.Request[datahubv1.MarkSummaryVersionSupersededRequest]) (*connect.Response[datahubv1.MarkSummaryVersionSupersededResponse], error) {
	f.gotSupersedeReq = req.Msg
	return connect.NewResponse(&datahubv1.MarkSummaryVersionSupersededResponse{
		PreviousVersion: f.prevVersion,
	}), nil
}

func (f *fakeVersionDataHubClient) GetSummaryVersionByID(_ context.Context, req *connect.Request[datahubv1.GetSummaryVersionByIDRequest]) (*connect.Response[datahubv1.GetSummaryVersionByIDResponse], error) {
	f.gotByIDReq = req.Msg
	return connect.NewResponse(&datahubv1.GetSummaryVersionByIDResponse{
		Version: f.byIDVersion,
	}), nil
}

func (f *fakeVersionDataHubClient) GetLatestSummaryVersion(_ context.Context, req *connect.Request[datahubv1.GetLatestSummaryVersionRequest]) (*connect.Response[datahubv1.GetLatestSummaryVersionResponse], error) {
	f.gotLatestReq = req.Msg
	return connect.NewResponse(&datahubv1.GetLatestSummaryVersionResponse{
		Version: f.latestVersion,
	}), nil
}

func TestVersionGateway_MarkSummaryVersionSuperseded_ForwardsUserID(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	newVersionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	prevVersionID := uuid.MustParse("44444444-4444-4444-8444-444444444444")

	client := &fakeVersionDataHubClient{
		prevVersion: &datahubv1.SummaryVersion{
			SummaryVersionId: prevVersionID.String(),
			ArticleId:        articleID.String(),
			UserId:           userID.String(),
			GeneratedAt:      timestamppb.New(time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)),
			Model:            "stream-summarize",
			SummaryText:      "older summary",
		},
	}
	gw := &VersionGateway{client: client}

	prev, err := gw.MarkSummaryVersionSuperseded(context.Background(), articleID, newVersionID, userID)
	require.NoError(t, err)
	require.NotNil(t, prev)
	assert.Equal(t, prevVersionID, prev.SummaryVersionID)
	require.NotNil(t, client.gotSupersedeReq)
	assert.Equal(t, articleID.String(), client.gotSupersedeReq.GetArticleId())
	assert.Equal(t, newVersionID.String(), client.gotSupersedeReq.GetNewVersionId())
	assert.Equal(t, userID.String(), client.gotSupersedeReq.GetUserId())
}

func TestVersionGateway_MarkSummaryVersionSuperseded_RequiresUserID(t *testing.T) {
	articleID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	newVersionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	client := &fakeVersionDataHubClient{}
	gw := &VersionGateway{client: client}

	_, err := gw.MarkSummaryVersionSuperseded(context.Background(), articleID, newVersionID, uuid.Nil)
	require.Error(t, err)
	assert.Nil(t, client.gotSupersedeReq)
}

func TestVersionGateway_GetLatestSummaryVersion_SendsUserIDFromContext(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	versionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	client := &fakeVersionDataHubClient{
		latestVersion: &datahubv1.SummaryVersion{
			SummaryVersionId: versionID.String(),
			ArticleId:        articleID.String(),
			UserId:           userID.String(),
			GeneratedAt:      timestamppb.New(time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)),
			Model:            "stream-summarize",
			SummaryText:      "latest summary",
		},
	}
	gw := &VersionGateway{client: client}

	sv, err := gw.GetLatestSummaryVersion(ctx, articleID)
	require.NoError(t, err)
	assert.Equal(t, versionID, sv.SummaryVersionID)
	require.NotNil(t, client.gotLatestReq)
	assert.Equal(t, articleID.String(), client.gotLatestReq.GetArticleId())
	assert.Equal(t, userID.String(), client.gotLatestReq.GetUserId())
}

func TestVersionGateway_GetLatestSummaryVersion_RequiresAuthenticatedUser(t *testing.T) {
	articleID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	client := &fakeVersionDataHubClient{}
	gw := &VersionGateway{client: client}

	_, err := gw.GetLatestSummaryVersion(context.Background(), articleID)
	require.Error(t, err)
	assert.Nil(t, client.gotLatestReq)
}

func TestVersionGateway_GetSummaryVersionByID_SendsUserIDFromContext(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	versionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	client := &fakeVersionDataHubClient{
		byIDVersion: &datahubv1.SummaryVersion{
			SummaryVersionId: versionID.String(),
			ArticleId:        articleID.String(),
			UserId:           userID.String(),
			GeneratedAt:      timestamppb.New(time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)),
			Model:            "stream-summarize",
			SummaryText:      "by-id summary",
		},
	}
	gw := &VersionGateway{client: client}

	sv, err := gw.GetSummaryVersionByID(ctx, versionID)
	require.NoError(t, err)
	assert.Equal(t, versionID, sv.SummaryVersionID)
	require.NotNil(t, client.gotByIDReq)
	assert.Equal(t, versionID.String(), client.gotByIDReq.GetSummaryVersionId())
	assert.Equal(t, userID.String(), client.gotByIDReq.GetUserId())
}

func TestVersionGateway_GetSummaryVersionByID_RequiresAuthenticatedUser(t *testing.T) {
	versionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	client := &fakeVersionDataHubClient{}
	gw := &VersionGateway{client: client}

	_, err := gw.GetSummaryVersionByID(context.Background(), versionID)
	require.Error(t, err)
	assert.Nil(t, client.gotByIDReq)
}
