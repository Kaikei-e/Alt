package job

import (
	"alt/domain"
	"alt/mocks"
	"alt/orchestrator/usecase/scraping_domain_usecase"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDailyScrapingPolicyJobRunner_InitialRun(t *testing.T) {
	// Initialize logger for tests
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDomainPort := mocks.NewMockScrapingDomainPort(ctrl)
	mockRobotsTxtPort := mocks.NewMockRobotsTxtPort(ctrl)
	usecase := scraping_domain_usecase.NewScrapingDomainUsecaseWithRobotsTxt(mockDomainPort, mockRobotsTxtPort)

	// Mock empty list to simulate no domains
	// EnsureDomainsFromFeedLinks will fail (no repository), but that's expected in tests
	// RefreshAllRobotsTxt will be called and should succeed with empty list
	mockDomainPort.EXPECT().
		List(gomock.Any(), 0, 50).
		Return([]*domain.ScrapingDomain{}, nil).
		Times(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start job in goroutine. DailyScrapingPolicyJobRunner runs its work
	// once and returns - it does not loop - so waiting on `done` with a
	// timeout already synchronizes on completion deterministically; no
	// extra sleep is needed before cancelling.
	done := make(chan bool)
	go func() {
		DailyScrapingPolicyJobRunner(ctx, usecase)
		done <- true
	}()

	// Wait for job to stop
	select {
	case <-done:
		// Job stopped successfully
	case <-time.After(1 * time.Second):
		t.Fatal("Job did not stop within timeout")
	}

	// Cancel context (job has already completed; this exercises the
	// cleanup path a caller would normally take).
	cancel()
}

func TestDailyScrapingPolicyJobRunner_ContextCancellation(t *testing.T) {
	// Initialize logger for tests
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDomainPort := mocks.NewMockScrapingDomainPort(ctrl)
	mockRobotsTxtPort := mocks.NewMockRobotsTxtPort(ctrl)
	usecase := scraping_domain_usecase.NewScrapingDomainUsecaseWithRobotsTxt(mockDomainPort, mockRobotsTxtPort)

	// Mock empty list
	mockDomainPort.EXPECT().
		List(gomock.Any(), 0, 50).
		Return([]*domain.ScrapingDomain{}, nil).
		AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	// Start job in goroutine. As above, DailyScrapingPolicyJobRunner is a
	// one-shot call, so synchronizing on `done` (instead of guessing a
	// sleep duration) already deterministically confirms it ran to
	// completion.
	done := make(chan bool)
	go func() {
		DailyScrapingPolicyJobRunner(ctx, usecase)
		done <- true
	}()

	// Wait for job to stop
	select {
	case <-done:
		// Job stopped successfully
	case <-time.After(1 * time.Second):
		t.Fatal("Job did not stop within timeout")
	}

	// Cancel context (job has already completed; exercises the cleanup
	// path a caller would normally take).
	cancel()
}

func TestScrapingPolicyRefreshInterval(t *testing.T) {
	// Verify the interval constant is set correctly
	assert.Equal(t, 24*time.Hour, ScrapingPolicyRefreshInterval)
}
