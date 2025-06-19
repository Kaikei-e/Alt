package fetch_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/usecase/testutil"
	"context"
	"errors"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestFetchFeedsListUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	tests := []struct {
		name      string
		ctx       context.Context
		mockSetup func()
		want      []*domain.FeedItem
		wantErr   bool
	}{
		{
			name: "success",
			ctx:  context.Background(),
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsList(gomock.Any()).Return(mockData, nil).Times(1)
			},
			want:    mockData,
			wantErr: false,
		},
		{
			name: "database error",
			ctx:  context.Background(),
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsList(gomock.Any()).Return(nil, testutil.ErrMockDatabase).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "cancelled context",
			ctx:  testutil.CreateCancelledContext(),
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsList(gomock.Any()).Return(nil, context.Canceled).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty result",
			ctx:  context.Background(),
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsList(gomock.Any()).Return(testutil.CreateEmptyFeedItems(), nil).Times(1)
			},
			want:    testutil.CreateEmptyFeedItems(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			u := &FetchFeedsListUsecase{
				fetchFeedsListGateway: mockGateway,
			}
			got, err := u.Execute(tt.ctx)
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

	mockGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	tests := []struct {
		name      string
		ctx       context.Context
		limit     int
		mockSetup func()
		want      []*domain.FeedItem
		wantErr   bool
	}{
		{
			name:  "success with valid limit",
			ctx:   context.Background(),
			limit: 10,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListLimit(gomock.Any(), 10).Return(mockData, nil).Times(1)
			},
			want:    mockData,
			wantErr: false,
		},
		{
			name:  "zero limit",
			ctx:   context.Background(),
			limit: 0,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListLimit(gomock.Any(), 0).Return(testutil.CreateEmptyFeedItems(), nil).Times(1)
			},
			want:    testutil.CreateEmptyFeedItems(),
			wantErr: false,
		},
		{
			name:  "negative limit",
			ctx:   context.Background(),
			limit: -1,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListLimit(gomock.Any(), -1).Return(nil, errors.New("invalid limit")).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:  "database error",
			ctx:   context.Background(),
			limit: 5,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListLimit(gomock.Any(), 5).Return(nil, testutil.ErrMockDatabase).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:  "cancelled context",
			ctx:   testutil.CreateCancelledContext(),
			limit: 5,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListLimit(gomock.Any(), 5).Return(nil, context.Canceled).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			u := &FetchFeedsListUsecase{
				fetchFeedsListGateway: mockGateway,
			}
			got, err := u.ExecuteLimit(tt.ctx, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchFeedsListUsecase.ExecuteLimit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchFeedsListUsecase.ExecuteLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchFeedsListUsecase_ExecutePage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := mocks.NewMockFetchFeedsPort(ctrl)
	mockData := testutil.CreateMockFeedItems()

	tests := []struct {
		name      string
		ctx       context.Context
		page      int
		mockSetup func()
		want      []*domain.FeedItem
		wantErr   bool
	}{
		{
			name: "success with valid page",
			ctx:  context.Background(),
			page: 1,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListPage(gomock.Any(), 1).Return(mockData, nil).Times(1)
			},
			want:    mockData,
			wantErr: false,
		},
		{
			name: "page zero",
			ctx:  context.Background(),
			page: 0,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListPage(gomock.Any(), 0).Return(testutil.CreateEmptyFeedItems(), nil).Times(1)
			},
			want:    testutil.CreateEmptyFeedItems(),
			wantErr: false,
		},
		{
			name: "negative page",
			ctx:  context.Background(),
			page: -1,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListPage(gomock.Any(), -1).Return(nil, errors.New("invalid page number")).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "large page number",
			ctx:  context.Background(),
			page: 999999,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListPage(gomock.Any(), 999999).Return(testutil.CreateEmptyFeedItems(), nil).Times(1)
			},
			want:    testutil.CreateEmptyFeedItems(),
			wantErr: false,
		},
		{
			name: "database error",
			ctx:  context.Background(),
			page: 1,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListPage(gomock.Any(), 1).Return(nil, testutil.ErrMockDatabase).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "cancelled context",
			ctx:  testutil.CreateCancelledContext(),
			page: 1,
			mockSetup: func() {
				mockGateway.EXPECT().FetchFeedsListPage(gomock.Any(), 1).Return(nil, context.Canceled).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			u := &FetchFeedsListUsecase{
				fetchFeedsListGateway: mockGateway,
			}
			got, err := u.ExecutePage(tt.ctx, tt.page)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchFeedsListUsecase.ExecutePage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchFeedsListUsecase.ExecutePage() = %v, want %v", got, tt.want)
			}
		})
	}
}
