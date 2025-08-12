package domain

import (
	"context"
	"time"
	"github.com/google/uuid"
)

type TenantStatus string
type SubscriptionTier string

const (
	TenantStatusActive   TenantStatus = "active"
	TenantStatusInactive TenantStatus = "inactive"
	TenantStatusSuspended TenantStatus = "suspended"
	
	SubscriptionTierFree       SubscriptionTier = "free"
	SubscriptionTierBasic      SubscriptionTier = "basic"
	SubscriptionTierPremium    SubscriptionTier = "premium"
	SubscriptionTierEnterprise SubscriptionTier = "enterprise"
)

type Tenant struct {
	ID               uuid.UUID                  `json:"id"`
	Name             string                     `json:"name"`
	Slug             string                     `json:"slug"`
	Description      string                     `json:"description"`
	Status           TenantStatus               `json:"status"`
	SubscriptionTier SubscriptionTier           `json:"subscription_tier"`
	MaxUsers         int                        `json:"max_users"`
	MaxFeeds         int                        `json:"max_feeds"`
	Settings         map[string]interface{}     `json:"settings"`
	CreatedAt        time.Time                  `json:"created_at"`
	UpdatedAt        time.Time                  `json:"updated_at"`
}

type TenantUpdates struct {
	Name         *string                    `json:"name,omitempty"`
	Description  *string                    `json:"description,omitempty"`
	Status       *TenantStatus              `json:"status,omitempty"`
	Settings     *map[string]interface{}    `json:"settings,omitempty"`
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