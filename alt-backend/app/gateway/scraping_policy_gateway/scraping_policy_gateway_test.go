package scraping_policy_gateway

import (
	"alt/domain"
	"alt/port/scraping_policy_port"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestScrapingPolicyGateway_CanFetchArticle_Allowed(t *testing.T) {
	// Setup mock repository
	mockRepo := new(MockScrapingDomainPort)

	// Mock: domain policy allows fetching
	mockDomain := &domain.ScrapingDomain{
		ID:                  uuid.New(),
		Domain:              "example.com",
		Scheme:              "https",
		AllowFetchBody:      true,
		AllowMLTraining:     true,
		AllowCacheDays:      7,
		ForceRespectRobots:  false,
		RobotsDisallowPaths: []string{},
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	mockRepo.On("GetByDomain", mock.Anything, "example.com").Return(mockDomain, nil)

	// Create gateway with mock
	gateway := &ScrapingPolicyGateway{
		scrapingDomainPort: mockRepo,
		lastRequestTime:    make(map[string]time.Time),
	}

	// Execute
	ctx := context.Background()
	canFetch, err := gateway.CanFetchArticle(ctx, "https://example.com/article/123")

	// Assert
	require.NoError(t, err)
	assert.True(t, canFetch)
	mockRepo.AssertExpectations(t)
}

func TestScrapingPolicyGateway_CanFetchArticle_NotAllowed(t *testing.T) {
	mockRepo := new(MockScrapingDomainPort)

	mockDomain := &domain.ScrapingDomain{
		ID:                  uuid.New(),
		Domain:              "example.com",
		Scheme:              "https",
		AllowFetchBody:      false, // Not allowed
		AllowMLTraining:     true,
		AllowCacheDays:      7,
		ForceRespectRobots:  false,
		RobotsDisallowPaths: []string{},
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	mockRepo.On("GetByDomain", mock.Anything, "example.com").Return(mockDomain, nil)

	gateway := &ScrapingPolicyGateway{
		scrapingDomainPort: mockRepo,
		lastRequestTime:    make(map[string]time.Time),
	}

	ctx := context.Background()
	canFetch, err := gateway.CanFetchArticle(ctx, "https://example.com/article/123")

	require.NoError(t, err)
	assert.False(t, canFetch)
	mockRepo.AssertExpectations(t)
}

func TestScrapingPolicyGateway_CanFetchArticle_DisallowedByRobotsTxt(t *testing.T) {
	mockRepo := new(MockScrapingDomainPort)

	mockDomain := &domain.ScrapingDomain{
		ID:                  uuid.New(),
		Domain:              "example.com",
		Scheme:              "https",
		AllowFetchBody:      true,
		AllowMLTraining:     true,
		AllowCacheDays:      7,
		ForceRespectRobots:  true, // Respect robots.txt
		RobotsDisallowPaths: []string{"/admin/", "/private/"},
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	mockRepo.On("GetByDomain", mock.Anything, "example.com").Return(mockDomain, nil)

	gateway := &ScrapingPolicyGateway{
		scrapingDomainPort: mockRepo,
		lastRequestTime:    make(map[string]time.Time),
	}

	ctx := context.Background()
	// Article URL matches disallowed path
	canFetch, err := gateway.CanFetchArticle(ctx, "https://example.com/admin/secret")

	require.NoError(t, err)
	assert.False(t, canFetch)
	mockRepo.AssertExpectations(t)
}

func TestScrapingPolicyGateway_CanFetchArticle_DefaultPolicy(t *testing.T) {
	mockRepo := new(MockScrapingDomainPort)

	// Mock: no policy exists, should create default
	mockRepo.On("GetByDomain", mock.Anything, "example.com").Return(nil, nil)
	mockRepo.On("Save", mock.Anything, mock.MatchedBy(func(sd *domain.ScrapingDomain) bool {
		return sd.Domain == "example.com" &&
			sd.AllowFetchBody == true &&
			sd.ForceRespectRobots == true
	})).Return(nil)

	gateway := &ScrapingPolicyGateway{
		scrapingDomainPort: mockRepo,
		lastRequestTime:    make(map[string]time.Time),
	}

	ctx := context.Background()
	canFetch, err := gateway.CanFetchArticle(ctx, "https://example.com/article/123")

	require.NoError(t, err)
	assert.True(t, canFetch) // Default allows fetching
	mockRepo.AssertExpectations(t)
}

func TestScrapingPolicyGateway_CanFetchArticle_EmptyHostname(t *testing.T) {
	// Test case for URL that parses successfully but has empty hostname
	t.Run("file:// URL", func(t *testing.T) {
		mockRepo := new(MockScrapingDomainPort)
		// GetByDomain should not be called for empty hostname
		// (validation happens before the call)

		gateway := &ScrapingPolicyGateway{
			scrapingDomainPort: mockRepo,
			lastRequestTime:    make(map[string]time.Time),
		}

		ctx := context.Background()
		canFetch, err := gateway.CanFetchArticle(ctx, "file:///path/to/file")

		// Empty hostname should return error and false
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no hostname")
		assert.False(t, canFetch)
		// Verify GetByDomain was not called
		mockRepo.AssertNotCalled(t, "GetByDomain")
	})

	// Test cases for URLs that fail to parse (these are caught earlier)
	testCases := []struct {
		name string
		url  string
	}{
		{
			name: "URL with only port",
			url:  ":80",
		},
		{
			name: "invalid URL without host",
			url:  "://path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo := new(MockScrapingDomainPort)
			gateway := &ScrapingPolicyGateway{
				scrapingDomainPort: mockRepo,
				lastRequestTime:    make(map[string]time.Time),
			}

			ctx := context.Background()
			canFetch, err := gateway.CanFetchArticle(ctx, tc.url)

			// Invalid URL should return error and false
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid article URL")
			assert.False(t, canFetch)
			// Verify GetByDomain was not called
			mockRepo.AssertNotCalled(t, "GetByDomain")
		})
	}
}

func TestScrapingPolicyGateway_ImplementsPort(t *testing.T) {
	// Verify that ScrapingPolicyGateway implements ScrapingPolicyPort interface
	var _ scraping_policy_port.ScrapingPolicyPort = (*ScrapingPolicyGateway)(nil)
}

// MockScrapingDomainPort is a mock implementation for testing
type MockScrapingDomainPort struct {
	mock.Mock
}

func (m *MockScrapingDomainPort) GetByDomain(ctx context.Context, domainName string) (*domain.ScrapingDomain, error) {
	args := m.Called(ctx, domainName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ScrapingDomain), args.Error(1)
}

func (m *MockScrapingDomainPort) GetByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ScrapingDomain), args.Error(1)
}

func (m *MockScrapingDomainPort) Save(ctx context.Context, sd *domain.ScrapingDomain) error {
	args := m.Called(ctx, sd)
	return args.Error(0)
}

func (m *MockScrapingDomainPort) List(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error) {
	args := m.Called(ctx, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.ScrapingDomain), args.Error(1)
}

func (m *MockScrapingDomainPort) UpdatePolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error {
	args := m.Called(ctx, id, update)
	return args.Error(0)
}
