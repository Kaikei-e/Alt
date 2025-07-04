package fetch_feed_stats_usecase

import (
	"alt/mocks"
	"alt/usecase/testutil"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestTodayUnreadArticlesCountUsecase_Execute(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockTodayUnreadArticlesCountPort(ctrl)

	tests := []struct {
		name      string
		ctx       context.Context
		since     time.Time
		mockSetup func()
		want      int
		wantErr   bool
	}{
		{
			name:      "success with positive count",
			ctx:       context.Background(),
			since:     time.Now(),
			mockSetup: func() { mockPort.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(5, nil).Times(1) },
			want:      5,
			wantErr:   false,
		},
		{
			name:  "database error",
			ctx:   context.Background(),
			since: time.Now(),
			mockSetup: func() {
				mockPort.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(0, testutil.ErrMockDatabase).Times(1)
			},
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			u := &TodayUnreadArticlesCountUsecase{todayUnreadArticlesCountPort: mockPort}
			got, err := u.Execute(tt.ctx, tt.since)
			if (err != nil) != tt.wantErr {
				t.Errorf("TodayUnreadArticlesCountUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TodayUnreadArticlesCountUsecase.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}
