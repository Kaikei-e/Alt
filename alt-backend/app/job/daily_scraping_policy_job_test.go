package job

import (
	"alt/domain"
	"alt/mocks"
	"alt/usecase/scraping_domain_usecase"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDailyScrapingPolicyJobRunner_InitialRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDomainPort := mocks.NewMockScrapingDomainPort(ctrl)
	mockRobotsTxtPort := mocks.NewMockRobotsTxtPort(ctrl)
	usecase := scraping_domain_usecase.NewScrapingDomainUsecaseWithRobotsTxt(mockDomainPort, mockRobotsTxtPort)

	// Mock empty list to simulate no domains
	mockDomainPort.EXPECT().
		List(gomock.Any(), 0, 50).
		Return([]*domain.ScrapingDomain{}, nil).
		Times(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start job in goroutine
	done := make(chan bool)
	go func() {
		DailyScrapingPolicyJobRunner(ctx, usecase)
		done <- true
	}()

	// Wait a bit to ensure initial run completes
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop job
	cancel()

	// Wait for job to stop
	select {
	case <-done:
		// Job stopped successfully
	case <-time.After(1 * time.Second):
		t.Fatal("Job did not stop within timeout")
	}
}

func TestDailyScrapingPolicyJobRunner_ContextCancellation(t *testing.T) {
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

	// Start job in goroutine
	done := make(chan bool)
	go func() {
		DailyScrapingPolicyJobRunner(ctx, usecase)
		done <- true
	}()

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for job to stop
	select {
	case <-done:
		// Job stopped successfully
	case <-time.After(1 * time.Second):
		t.Fatal("Job did not stop within timeout")
	}
}

func TestScrapingPolicyRefreshInterval(t *testing.T) {
	// Verify the interval constant is set correctly
	assert.Equal(t, 24*time.Hour, ScrapingPolicyRefreshInterval)
}
