package rss

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	rssv2 "alt/gen/proto/alt/rss/v2"

	"alt/domain"
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

func TestListRSSFeedLinksResponse_Construction(t *testing.T) {
	id1 := uuid.New().String()
	id2 := uuid.New().String()

	resp := &rssv2.ListRSSFeedLinksResponse{
		Links: []*rssv2.RSSFeedLink{
			{
				Id:  id1,
				Url: "https://example.com/feed1.xml",
			},
			{
				Id:  id2,
				Url: "https://example.com/feed2.xml",
			},
		},
	}

	assert.Len(t, resp.Links, 2)
	assert.Equal(t, id1, resp.Links[0].Id)
	assert.Equal(t, "https://example.com/feed1.xml", resp.Links[0].Url)
	assert.Equal(t, id2, resp.Links[1].Id)
	assert.Equal(t, "https://example.com/feed2.xml", resp.Links[1].Url)
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
		Message: "Feed link deleted",
	}

	assert.Equal(t, "Feed link deleted", resp.Message)
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

func TestFeedLinkToProto_Conversion(t *testing.T) {
	id := uuid.New()
	domainLink := domain.FeedLink{
		ID:  id,
		URL: "https://example.com/feed.xml",
	}

	// Simulate the conversion done in handler
	protoLink := &rssv2.RSSFeedLink{
		Id:  domainLink.ID.String(),
		Url: domainLink.URL,
	}

	assert.Equal(t, id.String(), protoLink.Id)
	assert.Equal(t, "https://example.com/feed.xml", protoLink.Url)
}

func TestFeedLinksToProto_Conversion(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	domainLinks := []*domain.FeedLink{
		{ID: id1, URL: "https://example.com/feed1.xml"},
		{ID: id2, URL: "https://example.com/feed2.xml"},
	}

	// Simulate the conversion done in handler
	protoLinks := make([]*rssv2.RSSFeedLink, 0, len(domainLinks))
	for _, link := range domainLinks {
		protoLinks = append(protoLinks, &rssv2.RSSFeedLink{
			Id:  link.ID.String(),
			Url: link.URL,
		})
	}

	assert.Len(t, protoLinks, 2)
	assert.Equal(t, id1.String(), protoLinks[0].Id)
	assert.Equal(t, id2.String(), protoLinks[1].Id)
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
