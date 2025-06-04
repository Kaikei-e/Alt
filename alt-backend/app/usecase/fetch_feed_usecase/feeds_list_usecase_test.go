package fetch_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/port/fetch_feed_port"
	"context"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestFetchFeedsListUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedsListGateway := mocks.NewMockFetchFeedsPort(ctrl)

	mockDomainFeedItem := []*domain.FeedItem{
		{
			Title:       "Test Feed 1",
			Description: "Test Description 1",
			Link:        "https://test.com/feed1",
		},
		{
			Title:       "Test Feed 2",
			Description: "Test Description 2",
			Link:        "https://test.com/feed2",
		},
	}

	type fields struct {
		fetchFeedsListGateway fetch_feed_port.FetchFeedsPort
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []*domain.FeedItem
		wantErr bool
	}{
		{
			name: "success",
			fields: fields{
				fetchFeedsListGateway: mockFetchFeedsListGateway,
			},
			args: args{
				ctx: context.Background(),
			},
			want:    mockDomainFeedItem,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFetchFeedsListGateway.EXPECT().FetchFeedsList(tt.args.ctx).Return(tt.want, nil).Times(1)
			u := &FetchFeedsListUsecase{
				fetchFeedsListGateway: tt.fields.fetchFeedsListGateway,
			}
			got, err := u.Execute(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchFeedsListUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchFeedsListUsecase.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchFeedsListUsecase_ExecuteLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetchFeedsListGateway := mocks.NewMockFetchFeedsPort(ctrl)

	mockDomainFeedItem := []*domain.FeedItem{
		{
			Title:       "Test Feed 1",
			Description: "Test Description 1",
			Link:        "https://test.com/feed1",
		},
		{
			Title:       "Test Feed 2",
			Description: "Test Description 2",
			Link:        "https://test.com/feed2",
		},
	}

	type fields struct {
		fetchFeedsListGateway fetch_feed_port.FetchFeedsPort
	}
	type args struct {
		ctx   context.Context
		limit int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []*domain.FeedItem
		wantErr bool
	}{
		{
			name: "success",
			fields: fields{
				fetchFeedsListGateway: mockFetchFeedsListGateway,
			},
			args: args{
				ctx:   context.Background(),
				limit: 1,
			},
			want:    mockDomainFeedItem,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFetchFeedsListGateway.EXPECT().FetchFeedsListLimit(tt.args.ctx, tt.args.limit).Return(tt.want, nil).Times(1)
			u := &FetchFeedsListUsecase{
				fetchFeedsListGateway: tt.fields.fetchFeedsListGateway,
			}
			got, err := u.ExecuteLimit(tt.args.ctx, tt.args.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchFeedsListUsecase.ExecuteOffset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchFeedsListUsecase.ExecuteOffset() = %v, want %v", got, tt.want)
			}
		})
	}
}
