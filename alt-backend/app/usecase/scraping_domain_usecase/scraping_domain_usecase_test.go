package scraping_domain_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TDD RED PHASE: Write failing tests first

func TestScrapingDomainUsecase_ListScrapingDomains_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockScrapingDomainPort(ctrl)
	usecase := NewScrapingDomainUsecase(mockPort)

	expectedDomains := []*domain.ScrapingDomain{
		{
			ID:                  uuid.New(),
			Domain:              "example.com",
			Scheme:              "https",
			AllowFetchBody:      true,
			AllowMLTraining:     true,
			AllowCacheDays:      7,
			ForceRespectRobots:  true,
			RobotsDisallowPaths: []string{},
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		},
	}

	mockPort.EXPECT().
		List(gomock.Any(), 0, 20).
		Return(expectedDomains, nil).
		Times(1)

	ctx := context.Background()
	result, err := usecase.ListScrapingDomains(ctx, 0, 20)

	require.NoError(t, err)
	assert.Equal(t, expectedDomains, result)
	assert.Len(t, result, 1)
}

func TestScrapingDomainUsecase_GetScrapingDomain_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockScrapingDomainPort(ctrl)
	usecase := NewScrapingDomainUsecase(mockPort)

	domainID := uuid.New()
	expectedDomain := &domain.ScrapingDomain{
		ID:                  domainID,
		Domain:              "example.com",
		Scheme:              "https",
		AllowFetchBody:      true,
		AllowMLTraining:     true,
		AllowCacheDays:      7,
		ForceRespectRobots:  true,
		RobotsDisallowPaths: []string{},
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	mockPort.EXPECT().
		GetByID(gomock.Any(), domainID).
		Return(expectedDomain, nil).
		Times(1)

	ctx := context.Background()
	result, err := usecase.GetScrapingDomain(ctx, domainID)

	require.NoError(t, err)
	assert.Equal(t, expectedDomain, result)
}

func TestScrapingDomainUsecase_UpdateScrapingDomainPolicy_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockScrapingDomainPort(ctrl)
	usecase := NewScrapingDomainUsecase(mockPort)

	domainID := uuid.New()
	update := &domain.ScrapingPolicyUpdate{
		AllowFetchBody:     boolPtr(false),
		ForceRespectRobots: boolPtr(true),
	}

	mockPort.EXPECT().
		UpdatePolicy(gomock.Any(), domainID, update).
		Return(nil).
		Times(1)

	ctx := context.Background()
	err := usecase.UpdateScrapingDomainPolicy(ctx, domainID, update)

	require.NoError(t, err)
}

func TestScrapingDomainUsecase_RefreshRobotsTxt_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDomainPort := mocks.NewMockScrapingDomainPort(ctrl)
	mockRobotsTxtPort := mocks.NewMockRobotsTxtPort(ctrl)
	usecase := NewScrapingDomainUsecaseWithRobotsTxt(mockDomainPort, mockRobotsTxtPort)

	domainID := uuid.New()
	existingDomain := &domain.ScrapingDomain{
		ID:                  domainID,
		Domain:              "example.com",
		Scheme:              "https",
		AllowFetchBody:      true,
		AllowMLTraining:     true,
		AllowCacheDays:      7,
		ForceRespectRobots:  true,
		RobotsDisallowPaths: []string{},
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	fetchedRobotsTxt := &domain.RobotsTxt{
		URL:           "https://example.com/robots.txt",
		Content:       "User-agent: *\nDisallow: /admin/",
		FetchedAt:     time.Now(),
		StatusCode:    200,
		CrawlDelay:    5,
		DisallowPaths: []string{"/admin/"},
	}

	mockDomainPort.EXPECT().
		GetByID(gomock.Any(), domainID).
		Return(existingDomain, nil).
		Times(1)

	mockRobotsTxtPort.EXPECT().
		FetchRobotsTxt(gomock.Any(), "example.com", "https").
		Return(fetchedRobotsTxt, nil).
		Times(1)

	mockDomainPort.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, sd *domain.ScrapingDomain) error {
			// Verify the saved domain has correct robots.txt data
			if sd.ID != domainID {
				return fmt.Errorf("unexpected domain ID")
			}
			if sd.RobotsTxtContent == nil || *sd.RobotsTxtContent != fetchedRobotsTxt.Content {
				return fmt.Errorf("unexpected robots.txt content")
			}
			if sd.RobotsCrawlDelaySec == nil || *sd.RobotsCrawlDelaySec != 5 {
				return fmt.Errorf("unexpected crawl delay")
			}
			if len(sd.RobotsDisallowPaths) != 1 || sd.RobotsDisallowPaths[0] != "/admin/" {
				return fmt.Errorf("unexpected disallow paths")
			}
			return nil
		}).
		Times(1)

	ctx := context.Background()
	err := usecase.RefreshRobotsTxt(ctx, domainID)

	require.NoError(t, err)
}

func TestScrapingDomainUsecase_RefreshRobotsTxt_DomainNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDomainPort := mocks.NewMockScrapingDomainPort(ctrl)
	mockRobotsTxtPort := mocks.NewMockRobotsTxtPort(ctrl)
	usecase := NewScrapingDomainUsecaseWithRobotsTxt(mockDomainPort, mockRobotsTxtPort)

	domainID := uuid.New()

	mockDomainPort.EXPECT().
		GetByID(gomock.Any(), domainID).
		Return(nil, nil).
		Times(1)

	ctx := context.Background()
	err := usecase.RefreshRobotsTxt(ctx, domainID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "scraping domain not found")
}

func TestScrapingDomainUsecase_RefreshAllRobotsTxt_Success(t *testing.T) {
	// Initialize logger for tests
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDomainPort := mocks.NewMockScrapingDomainPort(ctrl)
	mockRobotsTxtPort := mocks.NewMockRobotsTxtPort(ctrl)
	usecase := NewScrapingDomainUsecaseWithRobotsTxt(mockDomainPort, mockRobotsTxtPort)

	domainID1 := uuid.New()
	domainID2 := uuid.New()

	// First batch - only 2 domains, so pagination will stop after first call
	domainsBatch1 := []*domain.ScrapingDomain{
		{
			ID:                  domainID1,
			Domain:              "example.com",
			Scheme:              "https",
			AllowFetchBody:      true,
			AllowMLTraining:     true,
			AllowCacheDays:      7,
			ForceRespectRobots:  true,
			RobotsDisallowPaths: []string{},
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		},
		{
			ID:                  domainID2,
			Domain:              "test.com",
			Scheme:              "https",
			AllowFetchBody:      true,
			AllowMLTraining:     true,
			AllowCacheDays:      7,
			ForceRespectRobots:  true,
			RobotsDisallowPaths: []string{},
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		},
	}

	// Mock List call - only one call since len(domainsBatch1) < batchSize (2 < 50)
	mockDomainPort.EXPECT().
		List(gomock.Any(), 0, 50).
		Return(domainsBatch1, nil).
		Times(1)

	// Mock GetByID for each domain
	mockDomainPort.EXPECT().
		GetByID(gomock.Any(), domainID1).
		Return(domainsBatch1[0], nil).
		Times(1)

	mockDomainPort.EXPECT().
		GetByID(gomock.Any(), domainID2).
		Return(domainsBatch1[1], nil).
		Times(1)

	// Mock FetchRobotsTxt for each domain
	fetchedRobotsTxt1 := &domain.RobotsTxt{
		URL:           "https://example.com/robots.txt",
		Content:       "User-agent: *\nDisallow: /admin/",
		FetchedAt:     time.Now(),
		StatusCode:    200,
		CrawlDelay:    5,
		DisallowPaths: []string{"/admin/"},
	}

	fetchedRobotsTxt2 := &domain.RobotsTxt{
		URL:           "https://test.com/robots.txt",
		Content:       "User-agent: *\nAllow: /",
		FetchedAt:     time.Now(),
		StatusCode:    200,
		CrawlDelay:    0,
		DisallowPaths: []string{},
	}

	mockRobotsTxtPort.EXPECT().
		FetchRobotsTxt(gomock.Any(), "example.com", "https").
		Return(fetchedRobotsTxt1, nil).
		Times(1)

	mockRobotsTxtPort.EXPECT().
		FetchRobotsTxt(gomock.Any(), "test.com", "https").
		Return(fetchedRobotsTxt2, nil).
		Times(1)

	// Mock Save for each domain
	mockDomainPort.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(2)

	ctx := context.Background()
	err := usecase.RefreshAllRobotsTxt(ctx)

	require.NoError(t, err)
}

func TestScrapingDomainUsecase_RefreshAllRobotsTxt_NoRobotsTxtPort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDomainPort := mocks.NewMockScrapingDomainPort(ctrl)
	usecase := NewScrapingDomainUsecase(mockDomainPort) // Without robots.txt port

	ctx := context.Background()
	err := usecase.RefreshAllRobotsTxt(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "robots.txt port not available")
}

func TestScrapingDomainUsecase_RefreshAllRobotsTxt_PartialFailure(t *testing.T) {
	// Initialize logger for tests
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDomainPort := mocks.NewMockScrapingDomainPort(ctrl)
	mockRobotsTxtPort := mocks.NewMockRobotsTxtPort(ctrl)
	usecase := NewScrapingDomainUsecaseWithRobotsTxt(mockDomainPort, mockRobotsTxtPort)

	domainID1 := uuid.New()
	domainID2 := uuid.New()

	domainsBatch1 := []*domain.ScrapingDomain{
		{
			ID:                  domainID1,
			Domain:              "example.com",
			Scheme:              "https",
			AllowFetchBody:      true,
			AllowMLTraining:     true,
			AllowCacheDays:      7,
			ForceRespectRobots:  true,
			RobotsDisallowPaths: []string{},
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		},
		{
			ID:                  domainID2,
			Domain:              "test.com",
			Scheme:              "https",
			AllowFetchBody:      true,
			AllowMLTraining:     true,
			AllowCacheDays:      7,
			ForceRespectRobots:  true,
			RobotsDisallowPaths: []string{},
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		},
	}

	// Mock List call - only one call since len(domainsBatch1) < batchSize (2 < 50)
	mockDomainPort.EXPECT().
		List(gomock.Any(), 0, 50).
		Return(domainsBatch1, nil).
		Times(1)

	// First domain succeeds
	mockDomainPort.EXPECT().
		GetByID(gomock.Any(), domainID1).
		Return(domainsBatch1[0], nil).
		Times(1)

	fetchedRobotsTxt1 := &domain.RobotsTxt{
		URL:           "https://example.com/robots.txt",
		Content:       "User-agent: *\nDisallow: /admin/",
		FetchedAt:     time.Now(),
		StatusCode:    200,
		CrawlDelay:    5,
		DisallowPaths: []string{"/admin/"},
	}

	mockRobotsTxtPort.EXPECT().
		FetchRobotsTxt(gomock.Any(), "example.com", "https").
		Return(fetchedRobotsTxt1, nil).
		Times(1)

	mockDomainPort.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// Second domain fails
	mockDomainPort.EXPECT().
		GetByID(gomock.Any(), domainID2).
		Return(domainsBatch1[1], nil).
		Times(1)

	mockRobotsTxtPort.EXPECT().
		FetchRobotsTxt(gomock.Any(), "test.com", "https").
		Return(nil, fmt.Errorf("network error")).
		Times(1)

	ctx := context.Background()
	err := usecase.RefreshAllRobotsTxt(ctx)

	// Should not return error if at least one domain succeeded
	require.NoError(t, err)
}

func TestScrapingDomainUsecase_EnsureDomainsFromFeedLinks_NoRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDomainPort := mocks.NewMockScrapingDomainPort(ctrl)
	mockRobotsTxtPort := mocks.NewMockRobotsTxtPort(ctrl)
	usecase := NewScrapingDomainUsecaseWithRobotsTxt(mockDomainPort, mockRobotsTxtPort) // Without repository

	ctx := context.Background()
	err := usecase.EnsureDomainsFromFeedLinks(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "altDBRepository not available")
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
