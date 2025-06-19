package fetch_feed_stats_usecase

import (
	"alt/mocks"
	"alt/usecase/testutil"
	"alt/utils/logger"
	"context"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestFeedsCountUsecase_Execute(t *testing.T) {
	// Initialize logger to prevent nil pointer dereference
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockFeedAmountPort(ctrl)

	tests := []struct {
		name      string
		ctx       context.Context
		mockSetup func()
		want      int
		wantErr   bool
	}{
		{
			name: "success with positive count",
			ctx:  context.Background(),
			mockSetup: func() {
				mockPort.EXPECT().Execute(gomock.Any()).Return(10, nil).Times(1)
			},
			want:    10,
			wantErr: false,
		},
		{
			name: "success with zero count",
			ctx:  context.Background(),
			mockSetup: func() {
				mockPort.EXPECT().Execute(gomock.Any()).Return(0, nil).Times(1)
			},
			want:    0,
			wantErr: false,
		},
		{
			name: "success with large count",
			ctx:  context.Background(),
			mockSetup: func() {
				mockPort.EXPECT().Execute(gomock.Any()).Return(999999, nil).Times(1)
			},
			want:    999999,
			wantErr: false,
		},
		{
			name: "database error",
			ctx:  context.Background(),
			mockSetup: func() {
				mockPort.EXPECT().Execute(gomock.Any()).Return(0, testutil.ErrMockDatabase).Times(1)
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "context cancellation",
			ctx:  testutil.CreateCancelledContext(),
			mockSetup: func() {
				mockPort.EXPECT().Execute(gomock.Any()).Return(0, context.Canceled).Times(1)
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "timeout error",
			ctx:  context.Background(),
			mockSetup: func() {
				mockPort.EXPECT().Execute(gomock.Any()).Return(0, testutil.ErrMockTimeout).Times(1)
			},
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			u := &FeedsCountUsecase{
				feedsCountPort: mockPort,
			}
			got, err := u.Execute(tt.ctx)
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
