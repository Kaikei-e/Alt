package reading_status

import (
	"alt/mocks"
	"context"
	"errors"
	"net/url"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestFeedsReadingStatusUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFeedURL := url.URL{
		Scheme: "https",
		Host:   "example.com",
		Path:   "/feed",
	}
	mockFeedURLError := url.URL{}
	mockCtx := context.Background()
	mockUpdateFeedStatusGateway := mocks.NewMockUpdateFeedStatusPort(ctrl)

	type args struct {
		ctx     context.Context
		feedURL url.URL
	}
	tests := []struct {
		name    string
		u       *FeedsReadingStatusUsecase
		args    args
		wantErr bool
	}{
		{
			name: "success",
			u:    &FeedsReadingStatusUsecase{updateFeedStatusGateway: mockUpdateFeedStatusGateway},
			args: args{
				ctx:     mockCtx,
				feedURL: mockFeedURL,
			},
			wantErr: false,
		},
		{
			name: "error",
			u:    &FeedsReadingStatusUsecase{updateFeedStatusGateway: mockUpdateFeedStatusGateway},
			args: args{
				ctx:     mockCtx,
				feedURL: mockFeedURLError,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expectedError error
			if tt.wantErr {
				expectedError = errors.New("mock error")
			}
			mockUpdateFeedStatusGateway.EXPECT().UpdateFeedStatus(tt.args.ctx, tt.args.feedURL).Return(expectedError)

			u := tt.u
			if err := u.Execute(tt.args.ctx, tt.args.feedURL); (err != nil) != tt.wantErr {
				t.Errorf("FeedsReadingStatusUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
