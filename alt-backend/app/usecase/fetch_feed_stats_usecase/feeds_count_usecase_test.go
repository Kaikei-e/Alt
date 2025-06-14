package fetch_feed_stats_usecase

import (
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestFeedsCountUsecase_Execute(t *testing.T) {
	// Initialize logger to prevent nil pointer dereference
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	type fields struct {
		feedsCountPort *mocks.MockFeedAmountPort
	}
	type args struct {
		ctx       context.Context
		mockSetup func(*mocks.MockFeedAmountPort)
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "success",
			fields: fields{
				feedsCountPort: mocks.NewMockFeedAmountPort(ctrl),
			},
			args: args{
				ctx: ctx,
				mockSetup: func(mockPort *mocks.MockFeedAmountPort) {
					mockPort.EXPECT().Execute(ctx).Return(10, nil)
				},
			},
			want:    10,
			wantErr: false,
		},
		{
			name: "error",
			fields: fields{
				feedsCountPort: mocks.NewMockFeedAmountPort(ctrl),
			},
			args: args{
				ctx: ctx,
				mockSetup: func(mockPort *mocks.MockFeedAmountPort) {
					mockPort.EXPECT().Execute(ctx).Return(0, errors.New("error"))
				},
			},
			want:    0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPort := mocks.NewMockFeedAmountPort(ctrl)
			tt.args.mockSetup(mockPort)

			u := &FeedsCountUsecase{
				feedsCountPort: mockPort,
			}
			got, err := u.Execute(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("FeedsCountUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FeedsCountUsecase.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}
