package fetch_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"context"
	"reflect"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestFetchSingleFeedUsecase_Execute(t *testing.T) {
	// GoMockコントローラを初期化
	// このコントローラは、テスト終了時にMockオブジェクトの期待値を検証する
	ctrl := gomock.NewController(t)
	defer ctrl.Finish() // テスト終了時にVerify()を呼び出すことを保証
	ctx := context.Background()

	// 成功時の期待されるRSSFeedデータ
	mockSuccessFeed := &domain.RSSFeed{
		Title:         "Test Feed Title",
		Description:   "Description of test feed",
		Link:          "http://example.com/feed",
		FeedLink:      "http://example.com/feed.xml",
		Links:         []string{"http://example.com/article1", "http://example.com/article2"},
		Updated:       "2025-05-30T10:00:00Z",
		UpdatedParsed: time.Date(2025, time.May, 30, 10, 0, 0, 0, time.UTC),
		Language:      "en",
		Image: domain.RSSFeedImage{
			URL:   "http://example.com/image.png",
			Title: "Feed Image",
		},
		Generator: "Test Generator",
	}

	tests := []struct {
		name      string
		mockSetup func(*mocks.MockFetchSingleFeedPort)
		want      *domain.RSSFeed
		wantErr   bool
	}{
		{
			name: "success",
			mockSetup: func(mockPort *mocks.MockFetchSingleFeedPort) {
				mockPort.EXPECT().FetchSingleFeed(ctx).Return(mockSuccessFeed, nil).Times(1)
			},
			want:    mockSuccessFeed,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPort := mocks.NewMockFetchSingleFeedPort(ctrl)
			tt.mockSetup(mockPort)

			usecase := NewFetchSingleFeedUsecase(mockPort)
			got, err := usecase.Execute(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchSingleFeedUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchSingleFeedUsecase.Execute() got = %v, want %v", got, tt.want)
			}
		})
	}
}
