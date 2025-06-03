package register_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/port/fetch_feed_port"
	"alt/port/register_feed_port"
	"alt/utils/logger"
	"context"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestRegisterFeedUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	mockFetchFeedGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockRegisterFeedLinkPort := mocks.NewMockRegisterFeedLinkPort(ctrl)
	mockRegisterFeedsPort := mocks.NewMockRegisterFeedsPort(ctrl)

	type fields struct {
		registerFeedLinkGateway register_feed_port.RegisterFeedLinkPort
		registerFeedsGateway    register_feed_port.RegisterFeedsPort
		fetchFeedGateway        fetch_feed_port.FetchFeedsPort
	}
	type args struct {
		ctx   context.Context
		link  string
		feeds []*domain.FeedItem
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "success",
			fields: fields{
				registerFeedLinkGateway: mockRegisterFeedLinkPort,
				registerFeedsGateway:    mockRegisterFeedsPort,
				fetchFeedGateway:        mockFetchFeedGateway,
			},
			args: args{
				ctx:   ctx,
				link:  "https://www.usnews.com/rss/news",
				feeds: []*domain.FeedItem{},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegisterFeedLinkPort.EXPECT().RegisterRSSFeedLink(tt.args.ctx, tt.args.link).Return(nil).Times(1)
			mockFetchFeedGateway.EXPECT().FetchFeeds(tt.args.ctx, tt.args.link).Return(tt.args.feeds, nil).Times(1)
			mockRegisterFeedsPort.EXPECT().RegisterFeeds(tt.args.ctx, gomock.Any()).Return(nil).Times(1)

			r := NewRegisterFeedsUsecase(tt.fields.registerFeedLinkGateway, tt.fields.registerFeedsGateway, tt.fields.fetchFeedGateway)
			if err := r.Execute(tt.args.ctx, tt.args.link); (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
