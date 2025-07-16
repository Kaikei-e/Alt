package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"auth-service/app/domain"
)

func TestTenant_NewTenant(t *testing.T) {
	tests := []struct {
		name       string
		slug       string
		tenantName string
		wantErr    bool
	}{
		{
			name:       "valid tenant creation",
			slug:       "test-tenant",
			tenantName: "Test Tenant",
			wantErr:    false,
		},
		{
			name:       "empty slug",
			slug:       "",
			tenantName: "Test Tenant",
			wantErr:    true,
		},
		{
			name:       "empty name",
			slug:       "test-tenant",
			tenantName: "",
			wantErr:    true,
		},
		{
			name:       "invalid slug with spaces",
			slug:       "test tenant",
			tenantName: "Test Tenant",
			wantErr:    true,
		},
		{
			name:       "invalid slug with uppercase",
			slug:       "Test-Tenant",
			tenantName: "Test Tenant",
			wantErr:    true,
		},
		{
			name:       "slug too long",
			slug:       "this-is-a-very-long-tenant-slug-that-exceeds-the-maximum-allowed-length-of-100-characters-and-should-fail",
			tenantName: "Test Tenant",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant, err := domain.NewTenant(tt.slug, tt.tenantName)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tenant)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, tenant)
				assert.Equal(t, tt.slug, tenant.Slug)
				assert.Equal(t, tt.tenantName, tenant.Name)
				assert.Equal(t, domain.TenantStatusActive, tenant.Status)
				assert.False(t, tenant.CreatedAt.IsZero())
				assert.False(t, tenant.UpdatedAt.IsZero())
				assert.NotNil(t, tenant.Settings)
			}
		})
	}
}

func TestTenant_UpdateSettings(t *testing.T) {
	tenant, err := domain.NewTenant("test-tenant", "Test Tenant")
	require.NoError(t, err)

	originalUpdatedAt := tenant.UpdatedAt

	// Update settings
	newSettings := domain.TenantSettings{
		Features: []string{"rss_feeds", "ai_summary"},
		Limits: domain.TenantLimits{
			MaxFeeds: 500,
			MaxUsers: 10,
		},
		Timezone: "UTC",
		Language: "en",
	}

	err = tenant.UpdateSettings(newSettings)

	require.NoError(t, err)
	assert.Equal(t, newSettings.Features, tenant.Settings.Features)
	assert.Equal(t, newSettings.Limits.MaxFeeds, tenant.Settings.Limits.MaxFeeds)
	assert.Equal(t, newSettings.Limits.MaxUsers, tenant.Settings.Limits.MaxUsers)
	assert.Equal(t, newSettings.Timezone, tenant.Settings.Timezone)
	assert.Equal(t, newSettings.Language, tenant.Settings.Language)
	assert.True(t, tenant.UpdatedAt.After(originalUpdatedAt))
}

func TestTenant_Suspend(t *testing.T) {
	tenant, err := domain.NewTenant("test-tenant", "Test Tenant")
	require.NoError(t, err)

	// Should be active initially
	assert.True(t, tenant.IsActive())

	// Suspend tenant
	tenant.Suspend()

	assert.Equal(t, domain.TenantStatusSuspended, tenant.Status)
	assert.False(t, tenant.IsActive())
}

func TestTenant_Activate(t *testing.T) {
	tenant, err := domain.NewTenant("test-tenant", "Test Tenant")
	require.NoError(t, err)

	// Suspend first
	tenant.Suspend()
	assert.False(t, tenant.IsActive())

	// Activate
	tenant.Activate()

	assert.Equal(t, domain.TenantStatusActive, tenant.Status)
	assert.True(t, tenant.IsActive())
}

func TestTenant_SoftDelete(t *testing.T) {
	tenant, err := domain.NewTenant("test-tenant", "Test Tenant")
	require.NoError(t, err)

	// Should not be deleted initially
	assert.False(t, tenant.IsDeleted())

	// Soft delete
	tenant.SoftDelete()

	assert.Equal(t, domain.TenantStatusDeleted, tenant.Status)
	assert.True(t, tenant.IsDeleted())
	assert.NotNil(t, tenant.DeletedAt)
}

func TestTenant_HasFeature(t *testing.T) {
	tenant, err := domain.NewTenant("test-tenant", "Test Tenant")
	require.NoError(t, err)

	// Set features
	tenant.Settings.Features = []string{"rss_feeds", "ai_summary", "tags"}

	assert.True(t, tenant.HasFeature("rss_feeds"))
	assert.True(t, tenant.HasFeature("ai_summary"))
	assert.True(t, tenant.HasFeature("tags"))
	assert.False(t, tenant.HasFeature("non_existent_feature"))
}

func TestTenant_IsWithinLimits(t *testing.T) {
	tenant, err := domain.NewTenant("test-tenant", "Test Tenant")
	require.NoError(t, err)

	// Set limits
	tenant.Settings.Limits = domain.TenantLimits{
		MaxFeeds: 100,
		MaxUsers: 5,
	}

	tests := []struct {
		name      string
		userCount int
		feedCount int
		expected  bool
	}{
		{
			name:      "within limits",
			userCount: 3,
			feedCount: 50,
			expected:  true,
		},
		{
			name:      "at user limit",
			userCount: 5,
			feedCount: 50,
			expected:  true,
		},
		{
			name:      "at feed limit",
			userCount: 3,
			feedCount: 100,
			expected:  true,
		},
		{
			name:      "exceeds user limit",
			userCount: 6,
			feedCount: 50,
			expected:  false,
		},
		{
			name:      "exceeds feed limit",
			userCount: 3,
			feedCount: 101,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tenant.IsWithinLimits(tt.userCount, tt.feedCount)
			assert.Equal(t, tt.expected, result)
		})
	}
}
