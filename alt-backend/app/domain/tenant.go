package domain

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type TenantStatus string
type SubscriptionTier string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusInactive  TenantStatus = "inactive"
	TenantStatusSuspended TenantStatus = "suspended"

	SubscriptionTierFree       SubscriptionTier = "free"
	SubscriptionTierBasic      SubscriptionTier = "basic"
	SubscriptionTierPremium    SubscriptionTier = "premium"
	SubscriptionTierEnterprise SubscriptionTier = "enterprise"
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

type Tenant struct {
	ID               uuid.UUID        `json:"id"`
	Name             string           `json:"name"`
	Slug             string           `json:"slug"`
	Description      string           `json:"description"`
	Status           TenantStatus     `json:"status"`
	SubscriptionTier SubscriptionTier `json:"subscription_tier"`
	MaxUsers         int              `json:"max_users"`
	MaxFeeds         int              `json:"max_feeds"`
	Settings         TenantSettings   `json:"settings"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type TenantUpdates struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Status      *TenantStatus   `json:"status,omitempty"`
	Settings    *TenantSettings `json:"settings,omitempty"`
}

// TenantUsage represents current resource usage for a tenant
type TenantUsage struct {
	TenantID       uuid.UUID `json:"tenant_id"`
	UserCount      int       `json:"user_count"`
	FeedCount      int       `json:"feed_count"`
	StorageUsed    int64     `json:"storage_used"` // in bytes
	RequestsPerDay int       `json:"requests_per_day"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// コンテキストキー
type tenantContextKey string

const TenantContextKey tenantContextKey = "tenant_context"

// テナントコンテキストヘルパー関数
func SetTenantContext(ctx context.Context, tenant *Tenant) context.Context {
	return context.WithValue(ctx, TenantContextKey, tenant)
}

func GetTenantFromContext(ctx context.Context) (*Tenant, error) {
	tenant, ok := ctx.Value(TenantContextKey).(*Tenant)
	if !ok || tenant == nil {
		return nil, ErrTenantNotFound
	}
	return tenant, nil
}

// IsActive returns true if the tenant is active
func (t *Tenant) IsActive() bool {
	return t.Status == TenantStatusActive
}

// IsWithinLimits checks if the tenant is within its resource limits
func (t *Tenant) IsWithinLimits(userCount, feedCount int) bool {
	if userCount > t.MaxUsers {
		return false
	}
	if feedCount > t.MaxFeeds {
		return false
	}
	return true
}
