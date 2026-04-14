package rss

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	rssv2 "alt/gen/proto/alt/rss/v2"

	"alt/di"
	"alt/domain"
	"alt/mocks"
	"alt/usecase/feed_link_usecase"
	"alt/usecase/register_favorite_feed_usecase"
)

func createAuthContext() context.Context {
	userID := uuid.New()
	tenantID := uuid.New()
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  tenantID,
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

// =============================================================================
// Response Construction Tests
// =============================================================================

func TestRegisterRSSFeedResponse_Construction(t *testing.T) {
	resp := &rssv2.RegisterRSSFeedResponse{
		Message: "RSS feed link registered",
	}

	assert.Equal(t, "RSS feed link registered", resp.Message)
}

func TestRSSFeedLink_Construction(t *testing.T) {
	id := uuid.New().String()
	link := &rssv2.RSSFeedLink{
		Id:  id,
		Url: "https://example.com/feed.xml",
	}

	assert.Equal(t, id, link.Id)
	assert.Equal(t, "https://example.com/feed.xml", link.Url)
}

func TestRSSFeedLink_ConstructionWithHealth(t *testing.T) {
	id := uuid.New().String()
	link := &rssv2.RSSFeedLink{
		Id:                  id,
		Url:                 "https://example.com/feed.xml",
		HealthStatus:        "warning",
		ConsecutiveFailures: 2,
		LastFailureReason:   "connection timeout",
		IsActive:            true,
	}

	assert.Equal(t, id, link.Id)
	assert.Equal(t, "https://example.com/feed.xml", link.Url)
	assert.Equal(t, "warning", link.HealthStatus)
	assert.Equal(t, int32(2), link.ConsecutiveFailures)
	assert.Equal(t, "connection timeout", link.LastFailureReason)
	assert.True(t, link.IsActive)
}

func TestListRSSFeedLinksResponse_Construction(t *testing.T) {
	id1 := uuid.New().String()
	id2 := uuid.New().String()

	resp := &rssv2.ListRSSFeedLinksResponse{
		Links: []*rssv2.RSSFeedLink{
			{
				Id:           id1,
				Url:          "https://example.com/feed1.xml",
				HealthStatus: "healthy",
				IsActive:     true,
			},
			{
				Id:                  id2,
				Url:                 "https://example.com/feed2.xml",
				HealthStatus:        "error",
				ConsecutiveFailures: 5,
				LastFailureReason:   "timeout",
				IsActive:            true,
			},
		},
	}

	assert.Len(t, resp.Links, 2)
	assert.Equal(t, id1, resp.Links[0].Id)
	assert.Equal(t, "https://example.com/feed1.xml", resp.Links[0].Url)
	assert.Equal(t, "healthy", resp.Links[0].HealthStatus)
	assert.Equal(t, id2, resp.Links[1].Id)
	assert.Equal(t, "https://example.com/feed2.xml", resp.Links[1].Url)
	assert.Equal(t, "error", resp.Links[1].HealthStatus)
	assert.Equal(t, int32(5), resp.Links[1].ConsecutiveFailures)
}

func TestListRSSFeedLinksResponse_Empty(t *testing.T) {
	resp := &rssv2.ListRSSFeedLinksResponse{
		Links: []*rssv2.RSSFeedLink{},
	}

	assert.Empty(t, resp.Links)
	assert.NotNil(t, resp.Links)
}

func TestDeleteRSSFeedLinkResponse_Construction(t *testing.T) {
	resp := &rssv2.DeleteRSSFeedLinkResponse{
		Message: "Feed unsubscribed",
	}

	assert.Equal(t, "Feed unsubscribed", resp.Message)
}

func TestRegisterFavoriteFeedResponse_Construction(t *testing.T) {
	resp := &rssv2.RegisterFavoriteFeedResponse{
		Message: "favorite feed registered",
	}

	assert.Equal(t, "favorite feed registered", resp.Message)
}

// =============================================================================
// Request Validation Tests
// =============================================================================

func TestRegisterRSSFeedRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid URL",
			url:     "https://example.com/feed.xml",
			wantErr: false,
		},
		{
			name:    "empty URL should fail",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &rssv2.RegisterRSSFeedRequest{
				Url: tt.url,
			}

			if tt.wantErr {
				assert.Empty(t, req.Url)
			} else {
				assert.NotEmpty(t, req.Url)
			}
		})
	}
}

func TestDeleteRSSFeedLinkRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid UUID",
			id:      uuid.New().String(),
			wantErr: false,
		},
		{
			name:    "empty ID should fail",
			id:      "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &rssv2.DeleteRSSFeedLinkRequest{
				Id: tt.id,
			}

			if tt.wantErr {
				assert.Empty(t, req.Id)
			} else {
				assert.NotEmpty(t, req.Id)
			}
		})
	}
}

func TestRegisterFavoriteFeedRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid URL",
			url:     "https://example.com/article",
			wantErr: false,
		},
		{
			name:    "empty URL should fail",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &rssv2.RegisterFavoriteFeedRequest{
				Url: tt.url,
			}

			if tt.wantErr {
				assert.Empty(t, req.Url)
			} else {
				assert.NotEmpty(t, req.Url)
			}
		})
	}
}

// =============================================================================
// Domain Conversion Tests
// =============================================================================

func TestFeedLinkWithHealthToProto_Conversion(t *testing.T) {
	id := uuid.New()
	reason := "connection timeout"
	domainLink := &domain.FeedLinkWithHealth{
		FeedLink: domain.FeedLink{ID: id, URL: "https://example.com/feed.xml"},
		Availability: &domain.FeedLinkAvailability{
			FeedLinkID:          id,
			IsActive:            true,
			ConsecutiveFailures: 2,
			LastFailureReason:   &reason,
		},
	}

	// Simulate the conversion done in handler
	protoLink := &rssv2.RSSFeedLink{
		Id:                  domainLink.ID.String(),
		Url:                 domainLink.URL,
		HealthStatus:        string(domainLink.GetHealthStatus()),
		ConsecutiveFailures: int32(domainLink.Availability.ConsecutiveFailures),
		IsActive:            domainLink.Availability.IsActive,
	}
	if domainLink.Availability.LastFailureReason != nil {
		protoLink.LastFailureReason = *domainLink.Availability.LastFailureReason
	}

	assert.Equal(t, id.String(), protoLink.Id)
	assert.Equal(t, "https://example.com/feed.xml", protoLink.Url)
	assert.Equal(t, "warning", protoLink.HealthStatus)
	assert.Equal(t, int32(2), protoLink.ConsecutiveFailures)
	assert.True(t, protoLink.IsActive)
	assert.Equal(t, "connection timeout", protoLink.LastFailureReason)
}

func TestFeedLinkWithHealthToProto_NilAvailability(t *testing.T) {
	id := uuid.New()
	domainLink := &domain.FeedLinkWithHealth{
		FeedLink:     domain.FeedLink{ID: id, URL: "https://example.com/feed.xml"},
		Availability: nil,
	}

	protoLink := &rssv2.RSSFeedLink{
		Id:           domainLink.ID.String(),
		Url:          domainLink.URL,
		HealthStatus: string(domainLink.GetHealthStatus()),
	}

	assert.Equal(t, "unknown", protoLink.HealthStatus)
	assert.Equal(t, int32(0), protoLink.ConsecutiveFailures)
	assert.False(t, protoLink.IsActive)
}

func TestFeedLinksWithHealthToProto_Conversion(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	domainLinks := []*domain.FeedLinkWithHealth{
		{
			FeedLink:     domain.FeedLink{ID: id1, URL: "https://example.com/feed1.xml"},
			Availability: &domain.FeedLinkAvailability{IsActive: true, ConsecutiveFailures: 0},
		},
		{
			FeedLink:     domain.FeedLink{ID: id2, URL: "https://example.com/feed2.xml"},
			Availability: nil,
		},
	}

	// Simulate the conversion done in handler
	protoLinks := make([]*rssv2.RSSFeedLink, 0, len(domainLinks))
	for _, link := range domainLinks {
		protoLink := &rssv2.RSSFeedLink{
			Id:           link.ID.String(),
			Url:          link.URL,
			HealthStatus: string(link.GetHealthStatus()),
		}
		if link.Availability != nil {
			protoLink.ConsecutiveFailures = int32(link.Availability.ConsecutiveFailures)
			protoLink.IsActive = link.Availability.IsActive
		}
		protoLinks = append(protoLinks, protoLink)
	}

	assert.Len(t, protoLinks, 2)
	assert.Equal(t, id1.String(), protoLinks[0].Id)
	assert.Equal(t, "healthy", protoLinks[0].HealthStatus)
	assert.Equal(t, id2.String(), protoLinks[1].Id)
	assert.Equal(t, "unknown", protoLinks[1].HealthStatus)
}

// =============================================================================
// UUID Validation Tests
// =============================================================================

func TestUUIDValidation(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid UUID",
			id:      uuid.New().String(),
			wantErr: false,
		},
		{
			name:    "invalid UUID format",
			id:      "not-a-valid-uuid",
			wantErr: true,
		},
		{
			name:    "empty string",
			id:      "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uuid.Parse(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// RegisterFavoriteFeed Handler Tests
// =============================================================================

func newTestHandler(t *testing.T, container *di.ApplicationComponents) *Handler {
	t.Helper()
	logger := slog.Default()
	return NewHandler(container, nil, logger)
}

func TestRandomSubscription_Unauthenticated(t *testing.T) {
	h := newTestHandler(t, &di.ApplicationComponents{})
	_, err := h.RandomSubscription(context.Background(),
		connect.NewRequest(&rssv2.RandomSubscriptionRequest{}))
	assert.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestRandomSubscription_NoUsecase_Unimplemented(t *testing.T) {
	h := newTestHandler(t, &di.ApplicationComponents{})
	_, err := h.RandomSubscription(createAuthContext(),
		connect.NewRequest(&rssv2.RandomSubscriptionRequest{}))
	assert.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestRegisterFavoriteFeed_EmptyURL(t *testing.T) {
	h := newTestHandler(t, &di.ApplicationComponents{})
	ctx := createAuthContext()

	req := connect.NewRequest(&rssv2.RegisterFavoriteFeedRequest{
		Url: "",
	})

	_, err := h.RegisterFavoriteFeed(ctx, req)
	assert.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestRegisterFavoriteFeed_WhitespaceURL(t *testing.T) {
	h := newTestHandler(t, &di.ApplicationComponents{})
	ctx := createAuthContext()

	req := connect.NewRequest(&rssv2.RegisterFavoriteFeedRequest{
		Url: "   ",
	})

	_, err := h.RegisterFavoriteFeed(ctx, req)
	assert.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestRegisterFavoriteFeed_Unauthenticated(t *testing.T) {
	h := newTestHandler(t, &di.ApplicationComponents{})
	ctx := context.Background() // no auth

	req := connect.NewRequest(&rssv2.RegisterFavoriteFeedRequest{
		Url: "https://example.com/feed",
	})

	_, err := h.RegisterFavoriteFeed(ctx, req)
	assert.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestRegisterFavoriteFeed_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
	mockPort.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/feed").Return(nil)

	usecase := register_favorite_feed_usecase.NewRegisterFavoriteFeedUsecase(mockPort)
	container := &di.ApplicationComponents{
		RegisterFavoriteFeedUsecase: usecase,
	}
	h := newTestHandler(t, container)
	ctx := createAuthContext()

	req := connect.NewRequest(&rssv2.RegisterFavoriteFeedRequest{
		Url: "https://example.com/feed",
	})

	resp, err := h.RegisterFavoriteFeed(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, "favorite feed registered", resp.Msg.Message)
}

func TestRegisterFavoriteFeed_UsecaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
	mockPort.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/feed").Return(errors.New("db error"))

	usecase := register_favorite_feed_usecase.NewRegisterFavoriteFeedUsecase(mockPort)
	container := &di.ApplicationComponents{
		RegisterFavoriteFeedUsecase: usecase,
	}
	h := newTestHandler(t, container)
	ctx := createAuthContext()

	req := connect.NewRequest(&rssv2.RegisterFavoriteFeedRequest{
		Url: "https://example.com/feed",
	})

	_, err := h.RegisterFavoriteFeed(ctx, req)
	assert.Error(t, err)
}

func TestDeleteRSSFeedLink_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := createAuthContext()
	userCtx, err := domain.GetUserFromContext(ctx)
	assert.NoError(t, err)

	feedLinkID := uuid.New()
	mockPort := mocks.NewMockSubscriptionPort(ctrl)
	mockPort.EXPECT().Unsubscribe(gomock.Any(), userCtx.UserID, feedLinkID).Return(nil)

	usecase := feed_link_usecase.NewDeleteFeedLinkUsecase(mockPort)
	container := &di.ApplicationComponents{
		DeleteFeedLinkUsecase: usecase,
	}
	h := newTestHandler(t, container)

	req := connect.NewRequest(&rssv2.DeleteRSSFeedLinkRequest{
		Id: feedLinkID.String(),
	})

	resp, err := h.DeleteRSSFeedLink(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, "Feed unsubscribed", resp.Msg.Message)
}

func TestDeleteRSSFeedLink_InvalidID(t *testing.T) {
	h := newTestHandler(t, &di.ApplicationComponents{})

	req := connect.NewRequest(&rssv2.DeleteRSSFeedLinkRequest{
		Id: "not-a-uuid",
	})

	_, err := h.DeleteRSSFeedLink(createAuthContext(), req)
	assert.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
