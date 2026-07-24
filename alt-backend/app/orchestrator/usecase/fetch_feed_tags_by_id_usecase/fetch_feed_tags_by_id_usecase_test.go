package fetch_feed_tags_by_id_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

// Finding [10]: RestHandleFetchFeedTagsByID called container.AltDBRepository
// (a driver-layer type) directly from the handler, the sole remaining
// Handler->Driver layer violation in details.go. FetchFeedTagsByIDUsecase
// closes it by giving the handler a usecase to call, delegating to the
// existing fetch_feed_tags_port.FetchFeedTagsPort (already implemented by
// fetch_feed_tags_gateway and already used, via a URL-based usecase, by
// RestHandleFetchFeedTags in the same file).
func TestFetchFeedTagsByIDUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := mocks.NewMockFetchFeedTagsPort(ctrl)

	mockTags := []*domain.FeedTag{
		{ID: "1", TagName: "Technology", CreatedAt: time.Now()},
	}
	feedID := "feed-123"

	tests := []struct {
		name      string
		feedID    string
		cursor    *time.Time
		limit     int
		mockSetup func()
		want      []*domain.FeedTag
		wantErr   bool
	}{
		{
			name:   "success - delegates to gateway with feedID",
			feedID: feedID,
			cursor: nil,
			limit:  20,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedTags(gomock.Any(), feedID, nil, 20).Return(mockTags, nil).Times(1)
			},
			want:    mockTags,
			wantErr: false,
		},
		{
			name:      "invalid - empty feedID rejected without calling gateway",
			feedID:    "",
			cursor:    nil,
			limit:     20,
			mockSetup: func() {},
			want:      nil,
			wantErr:   true,
		},
		{
			name:   "error - gateway failure propagates",
			feedID: feedID,
			cursor: nil,
			limit:  20,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedTags(gomock.Any(), feedID, nil, 20).Return(nil, errors.New("db error")).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			u := NewFetchFeedTagsByIDUsecase(mockGateway)
			got, err := u.Execute(context.Background(), tt.feedID, tt.cursor, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}
