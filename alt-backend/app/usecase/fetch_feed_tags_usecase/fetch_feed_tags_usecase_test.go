package fetch_feed_tags_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestFetchFeedTagsUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedTagsGateway := mocks.NewMockFetchFeedTagsPort(ctrl)
	mockFeedURLToIDGateway := mocks.NewMockFeedURLToIDPort(ctrl)

	// Mock data for testing
	mockTags := []*domain.FeedTag{
		{
			ID:        1,
			Name:      "Technology",
			CreatedAt: time.Now(),
		},
		{
			ID:        2,
			Name:      "Programming",
			CreatedAt: time.Now(),
		},
	}

	feedURL := "https://example.com/rss.xml"
	feedID := uuid.New().String()

	tests := []struct {
		name      string
		ctx       context.Context
		feedURL   string
		cursor    *time.Time
		limit     int
		mockSetup func()
		want      []*domain.FeedTag
		wantErr   bool
	}{
		{
			name:    "success - normal case (no cursor)",
			ctx:     context.Background(),
			feedURL: feedURL,
			cursor:  nil,
			limit:   20,
			mockSetup: func() {
				mockFeedURLToIDGateway.EXPECT().GetFeedIDByURL(gomock.Any(), feedURL).Return(feedID, nil).Times(1)
				mockFetchFeedTagsGateway.EXPECT().FetchFeedTags(gomock.Any(), feedID, nil, 20).Return(mockTags, nil).Times(1)
			},
			want:    mockTags,
			wantErr: false,
		},
		{
			name:    "success - small limit",
			ctx:     context.Background(),
			feedURL: feedURL,
			cursor:  nil,
			limit:   5,
			mockSetup: func() {
				mockFeedURLToIDGateway.EXPECT().GetFeedIDByURL(gomock.Any(), feedURL).Return(feedID, nil).Times(1)
				mockFetchFeedTagsGateway.EXPECT().FetchFeedTags(gomock.Any(), feedID, nil, 5).Return(mockTags[:1], nil).Times(1)
			},
			want:    mockTags[:1],
			wantErr: false,
		},
		{
			name:    "success - with cursor",
			ctx:     context.Background(),
			feedURL: feedURL,
			cursor:  &time.Time{},
			limit:   20,
			mockSetup: func() {
				mockFeedURLToIDGateway.EXPECT().GetFeedIDByURL(gomock.Any(), feedURL).Return(feedID, nil).Times(1)
				mockFetchFeedTagsGateway.EXPECT().FetchFeedTags(gomock.Any(), feedID, gomock.Any(), 20).Return(mockTags[:1], nil).Times(1)
			},
			want:    mockTags[:1],
			wantErr: false,
		},
		{
			name:    "success - empty result",
			ctx:     context.Background(),
			feedURL: feedURL,
			cursor:  nil,
			limit:   20,
			mockSetup: func() {
				mockFeedURLToIDGateway.EXPECT().GetFeedIDByURL(gomock.Any(), feedURL).Return(feedID, nil).Times(1)
				mockFetchFeedTagsGateway.EXPECT().FetchFeedTags(gomock.Any(), feedID, nil, 20).Return([]*domain.FeedTag{}, nil).Times(1)
			},
			want:    []*domain.FeedTag{},
			wantErr: false,
		},
		{
			name:    "invalid feed_url - empty",
			ctx:     context.Background(),
			feedURL: "",
			cursor:  nil,
			limit:   20,
			mockSetup: func() {
				// Should not call any gateway for invalid feedURL
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid limit - zero",
			ctx:     context.Background(),
			feedURL: feedURL,
			cursor:  nil,
			limit:   0,
			mockSetup: func() {
				// Should not call any gateway for invalid limit
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid limit - negative",
			ctx:     context.Background(),
			feedURL: feedURL,
			cursor:  nil,
			limit:   -1,
			mockSetup: func() {
				// Should not call any gateway for invalid limit
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid limit - too large",
			ctx:     context.Background(),
			feedURL: feedURL,
			cursor:  nil,
			limit:   101,
			mockSetup: func() {
				// Should not call any gateway for limit > 100
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "error - feed URL to ID conversion fails",
			ctx:     context.Background(),
			feedURL: feedURL,
			cursor:  nil,
			limit:   20,
			mockSetup: func() {
				mockFeedURLToIDGateway.EXPECT().GetFeedIDByURL(gomock.Any(), feedURL).Return("", errors.New("feed not found")).Times(1)
				// Should not call fetch tags gateway when URL conversion fails
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "error - fetch tags fails",
			ctx:     context.Background(),
			feedURL: feedURL,
			cursor:  nil,
			limit:   20,
			mockSetup: func() {
				mockFeedURLToIDGateway.EXPECT().GetFeedIDByURL(gomock.Any(), feedURL).Return(feedID, nil).Times(1)
				mockFetchFeedTagsGateway.EXPECT().FetchFeedTags(gomock.Any(), feedID, nil, 20).Return(nil, errors.New("database error")).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			u := &FetchFeedTagsUsecase{
				feedURLToIDGateway:   mockFeedURLToIDGateway,
				fetchFeedTagsGateway: mockFetchFeedTagsGateway,
			}
			got, err := u.Execute(tt.ctx, tt.feedURL, tt.cursor, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchFeedTagsUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchFeedTagsUsecase.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}
