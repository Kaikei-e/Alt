package fetch_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/usecase/testutil"
	"alt/utils/logger"
	"context"
	"reflect"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestFetchUnreadFeedsListCursorUsecase_Execute(t *testing.T) {
	// Initialize logger to prevent nil pointer dereference
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	// Create cursor time for testing
	cursorTime := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name        string
		ctx         context.Context
		cursor      *time.Time
		limit       int
		mockSetup   func()
		want        []*domain.FeedItem
		wantHasMore bool
		wantErr     bool
	}{
		{
			name:   "success - first page (no cursor)",
			ctx:    context.Background(),
			cursor: nil,
			limit:  10,
			mockSetup: func() {
				mockGateway.EXPECT().FetchUnreadFeedsListCursor(gomock.Any(), gomock.Any(), 11).Return(mockData, nil).Times(1)
			},
			want:        mockData,
			wantHasMore: false,
			wantErr:     false,
		},
		{
			name:   "success - with cursor",
			ctx:    context.Background(),
			cursor: &cursorTime,
			limit:  10,
			mockSetup: func() {
				mockGateway.EXPECT().FetchUnreadFeedsListCursor(gomock.Any(), gomock.Any(), 11).Return(mockData, nil).Times(1)
			},
			want:        mockData,
			wantHasMore: false,
			wantErr:     false,
		},
		{
			name:   "success - has more pages",
			ctx:    context.Background(),
			cursor: nil,
			limit:  1,
			mockSetup: func() {
				mockGateway.EXPECT().
					FetchUnreadFeedsListCursor(gomock.Any(), gomock.Any(), 2).
					Return(mockData, nil).Times(1)
			},
			want:        mockData[:1],
			wantHasMore: true,
			wantErr:     false,
		},
		{
			name:   "success - limit 20 with has more",
			ctx:    context.Background(),
			cursor: nil,
			limit:  20,
			mockSetup: func() {
				// Return 21 items to indicate has_more=true
				extendedData := make([]*domain.FeedItem, 21)
				for i := 0; i < 21; i++ {
					extendedData[i] = mockData[i%len(mockData)]
				}
				mockGateway.EXPECT().
					FetchUnreadFeedsListCursor(gomock.Any(), gomock.Any(), 21).
					Return(extendedData, nil).Times(1)
			},
			want: func() []*domain.FeedItem {
				// Create 20 items for want
				result := make([]*domain.FeedItem, 20)
				for i := 0; i < 20; i++ {
					result[i] = mockData[i%len(mockData)]
				}
				return result
			}(),
			wantHasMore: true,
			wantErr:     false,
		},
		{
			name:   "success - empty result",
			ctx:    context.Background(),
			cursor: &cursorTime,
			limit:  10,
			mockSetup: func() {
				mockGateway.EXPECT().
					FetchUnreadFeedsListCursor(gomock.Any(), gomock.Any(), 11).
					Return(testutil.CreateEmptyFeedItems(), nil).Times(1)
			},
			want:        testutil.CreateEmptyFeedItems(),
			wantHasMore: false,
			wantErr:     false,
		},
		{
			name:   "invalid limit - zero",
			ctx:    context.Background(),
			cursor: nil,
			limit:  0,
			mockSetup: func() {
				// Should not call gateway for invalid limit
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid limit - negative",
			ctx:    context.Background(),
			cursor: nil,
			limit:  -1,
			mockSetup: func() {
				// Should not call gateway for invalid limit
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid limit - too large",
			ctx:    context.Background(),
			cursor: nil,
			limit:  101,
			mockSetup: func() {
				// Should not call gateway for limit > 100
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "database error",
			ctx:    context.Background(),
			cursor: nil,
			limit:  10,
			mockSetup: func() {
				mockGateway.EXPECT().FetchUnreadFeedsListCursor(gomock.Any(), gomock.Any(), 11).Return(nil, testutil.ErrMockDatabase).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "cancelled context",
			ctx:    testutil.CreateCancelledContext(),
			cursor: nil,
			limit:  10,
			mockSetup: func() {
				mockGateway.EXPECT().FetchUnreadFeedsListCursor(gomock.Any(), gomock.Any(), 11).Return(nil, context.Canceled).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			u := &FetchUnreadFeedsListCursorUsecase{
				fetchFeedsListGateway: mockGateway,
			}
			got, hasMore, err := u.Execute(tt.ctx, tt.cursor, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchUnreadFeedsListCursorUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchUnreadFeedsListCursorUsecase.Execute() = %v, want %v", got, tt.want)
			}
			if hasMore != tt.wantHasMore {
				t.Errorf("FetchUnreadFeedsListCursorUsecase.Execute() hasMore = %v, want %v", hasMore, tt.wantHasMore)
			}
		})
	}
}
