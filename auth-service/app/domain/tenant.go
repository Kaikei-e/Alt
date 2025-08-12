package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TenantStatus represents the status of a tenant
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusDeleted   TenantStatus = "deleted"
)

// SubscriptionTier represents different subscription levels
type SubscriptionTier string

const (
	SubscriptionTierFree     SubscriptionTier = "free"
	SubscriptionTierBasic    SubscriptionTier = "basic"
	SubscriptionTierPremium  SubscriptionTier = "premium"
	SubscriptionTierBusiness SubscriptionTier = "business"
)

// TenantLimits defines resource limits for a tenant
type TenantLimits struct {
	MaxFeeds int `json:"max_feeds"`
	MaxUsers int `json:"max_users"`
}

// TenantSettings holds tenant-specific configuration
type TenantSettings struct {
	Features []string     `json:"features"`
	Limits   TenantLimits `json:"limits"`
	Timezone string       `json:"timezone"`
	Language string       `json:"language"`
}

// Tenant represents a tenant in the multi-tenant system
type Tenant struct {
	ID               uuid.UUID        `json:"id"`
	Slug             string           `json:"slug"`
	Name             string           `json:"name"`
	Description      string           `json:"description,omitempty"`
	Status           TenantStatus     `json:"status"`
	SubscriptionTier SubscriptionTier `json:"subscription_tier"`
	MaxUsers         int              `json:"max_users"`
	Settings         TenantSettings   `json:"settings"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	DeletedAt        *time.Time       `json:"deleted_at,omitempty"`
}

// slugRegex validates tenant slugs (lowercase, alphanumeric, hyphens only)
var slugRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// NewTenant creates a new tenant with validation
func NewTenant(slug, name string) (*Tenant, error) {
	// Validate slug
	if slug == "" {
		return nil, fmt.Errorf("slug is required")
	}

	if len(slug) > 100 {
		return nil, fmt.Errorf("slug must be 100 characters or less")
	}

	if !slugRegex.MatchString(slug) {
		return nil, fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens")
	}

	// Validate name
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty or whitespace only")
	}

	now := time.Now()

	tenant := &Tenant{
		ID:     uuid.New(),
		Slug:   slug,
		Name:   name,
		Status: TenantStatusActive,
		Settings: TenantSettings{
			Features: []string{"rss_feeds", "ai_summary", "tags"},
			Limits: TenantLimits{
				MaxFeeds: 1000,
				MaxUsers: 50,
			},
			Timezone: "Asia/Tokyo",
			Language: "ja",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	return tenant, nil
}

// UpdateSettings updates the tenant settings
func (t *Tenant) UpdateSettings(settings TenantSettings) error {
	t.Settings = settings
	t.UpdatedAt = time.Now()
	return nil
}

// Suspend suspends the tenant
func (t *Tenant) Suspend() {
	t.Status = TenantStatusSuspended
	t.UpdatedAt = time.Now()
}

// Activate activates the tenant
func (t *Tenant) Activate() {
	t.Status = TenantStatusActive
	t.UpdatedAt = time.Now()
}

// SoftDelete marks the tenant as deleted
func (t *Tenant) SoftDelete() {
	now := time.Now()
	t.DeletedAt = &now
	t.Status = TenantStatusDeleted
	t.UpdatedAt = now
}

// IsActive returns true if the tenant is active
func (t *Tenant) IsActive() bool {
	return t.Status == TenantStatusActive
}

// IsDeleted returns true if the tenant is soft deleted
func (t *Tenant) IsDeleted() bool {
	return t.DeletedAt != nil || t.Status == TenantStatusDeleted
}

// HasFeature checks if the tenant has a specific feature enabled
func (t *Tenant) HasFeature(feature string) bool {
	for _, f := range t.Settings.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// IsWithinLimits checks if the tenant is within its resource limits
func (t *Tenant) IsWithinLimits(userCount, feedCount int) bool {
	if userCount > t.Settings.Limits.MaxUsers {
		return false
	}
	if feedCount > t.Settings.Limits.MaxFeeds {
		return false
	}
	return true
}

// CreateTenantRequest represents tenant creation request
type CreateTenantRequest struct {
	Slug string `json:"slug" validate:"required,alpha_dash,min=3,max=50"`
	Name string `json:"name" validate:"required,min=3,max=100"`
}

// TenantUsage represents current resource usage for a tenant
type TenantUsage struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	UserCount   int       `json:"user_count"`
	FeedCount   int       `json:"feed_count"`
	StorageUsed int64     `json:"storage_used"` // in bytes
	RequestsPerDay int    `json:"requests_per_day"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UpdateName updates the tenant name
func (t *Tenant) UpdateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name cannot be empty or whitespace only")
	}

	t.Name = name
	t.UpdatedAt = time.Now()
	return nil
}

// TenantUpdates represents updates to apply to a tenant
type TenantUpdates struct {
	Name             *string           `json:"name,omitempty"`
	Description      *string           `json:"description,omitempty"`
	SubscriptionTier *SubscriptionTier `json:"subscription_tier,omitempty"`
	MaxUsers         *int              `json:"max_users,omitempty"`
	Settings         *TenantSettings   `json:"settings,omitempty"`
}

