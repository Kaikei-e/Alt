package fetch_feed_details_usecase

import (
	"alt/domain"
	"alt/mocks"
	apperrors "alt/utils/errors"
	"context"
	"errors"
	"net/url"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestFeedsSummaryUsecase_Execute(t *testing.T) {
	// GoMockコントローラを初期化
	// このコントローラは、テスト終了時にMockオブジェクトの期待値を検証する
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	type args struct {
		ctx     context.Context
		feedURL *url.URL
	}
	tests := []struct {
		name      string
		mockSetup func(*mocks.MockFetchFeedDetailsPort)
		args      args
		want      *domain.FeedSummary
		wantErr   bool
	}{
		{
			name: "success",
			mockSetup: func(mockPort *mocks.MockFetchFeedDetailsPort) {
				mockPort.EXPECT().FetchFeedDetails(ctx, gomock.Any()).Return(&domain.FeedSummary{
					Summary: "Test Feed Summary",
				}, nil).Times(1)
			},
			args: args{
				ctx:     ctx,
				feedURL: &url.URL{Scheme: "http", Host: "example.com", Path: "/feed"},
			},
			want: &domain.FeedSummary{
				Summary: "Test Feed Summary",
			},
			wantErr: false,
		},
		{
			name: "error",
			mockSetup: func(mockPort *mocks.MockFetchFeedDetailsPort) {
				mockPort.EXPECT().FetchFeedDetails(ctx, gomock.Any()).Return(nil, errors.New("fetch error")).Times(1)
			},
			args: args{
				ctx:     ctx,
				feedURL: &url.URL{Scheme: "http", Host: "example.com", Path: "/feed"},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFetchFeedDetailsPort := mocks.NewMockFetchFeedDetailsPort(ctrl)
			tt.mockSetup(mockFetchFeedDetailsPort)

			u := &FeedsSummaryUsecase{
				fetchFeedDetailsPort: mockFetchFeedDetailsPort,
			}
			got, err := u.Execute(tt.args.ctx, tt.args.feedURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("FeedsSummaryUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FeedsSummaryUsecase.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A (nil, nil) port result is the data-hub gateway's "no summary generated
// yet" contract (unset field over RPC). At this endpoint's altitude that is
// a not-found, not a success: returning it as-is made the REST handler emit
// 200 with a null body, which e2e 24-feeds-details-tags.hurl exists to catch.
func TestFeedsSummaryUsecase_Execute_NilSummaryIsTypedNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	mockPort := mocks.NewMockFetchFeedDetailsPort(ctrl)
	mockPort.EXPECT().FetchFeedDetails(ctx, gomock.Any()).Return(nil, nil).Times(1)

	u := NewFeedsSummaryUsecase(mockPort)
	got, err := u.Execute(ctx, &url.URL{Scheme: "http", Host: "example.com", Path: "/feed"})
	if got != nil {
		t.Fatalf("expected nil summary, got %v", got)
	}
	if !errors.Is(err, apperrors.ErrFeedNotFound) {
		t.Fatalf("expected ErrFeedNotFound, got %v", err)
	}
}
