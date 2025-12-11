package dashboard_usecase

import (
	"context"
	"testing"
	"time"

	"alt/domain"
	"alt/mocks"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetRecapJobs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockRecapJobRepository(ctrl)
	usecase := NewGetRecapJobsUsecase(mockRepo)

	ctx := context.Background()
	now := time.Now()
	expectedJobs := []domain.RecapJob{
		{
			JobID:     "job-1",
			Status:    "completed",
			KickedAt:  now,
			UpdatedAt: now,
		},
	}

	mockRepo.EXPECT().
		GetRecapJobs(ctx, int64(3600), int64(100)).
		Return(expectedJobs, nil)

	jobs, err := usecase.Execute(ctx, 3600, 100)
	assert.NoError(t, err)
	assert.Equal(t, expectedJobs, jobs)
}
