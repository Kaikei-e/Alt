package port

import (
	"context"

	"auth-service/app/domain"
	"github.com/google/uuid"
)

// TenantUsecase defines tenant management business logic interface
type TenantUsecase interface {
	// Tenant management
	CreateTenant(ctx context.Context, req *domain.CreateTenantRequest) (*domain.Tenant, error)
	GetTenantByID(ctx context.Context, tenantID uuid.UUID) (*domain.Tenant, error)
	GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	UpdateTenant(ctx context.Context, tenantID uuid.UUID, updates *domain.Tenant) error
	DeleteTenant(ctx context.Context, tenantID uuid.UUID) error

	// Tenant queries
	ListTenants(ctx context.Context, limit, offset int) ([]*domain.Tenant, error)
	SearchTenants(ctx context.Context, query string, limit, offset int) ([]*domain.Tenant, error)

	// Tenant operations
	SuspendTenant(ctx context.Context, tenantID uuid.UUID) error
	ActivateTenant(ctx context.Context, tenantID uuid.UUID) error
	UpdateTenantSettings(ctx context.Context, tenantID uuid.UUID, settings domain.TenantSettings) error

	// Resource management
	CheckTenantLimits(ctx context.Context, tenantID uuid.UUID, userCount, feedCount int) error
	GetTenantUsage(ctx context.Context, tenantID uuid.UUID) (*domain.TenantUsage, error)
}

// TenantGateway defines tenant gateway interface
type TenantGateway interface {
	// Tenant operations
	CreateTenant(ctx context.Context, tenant *domain.Tenant) error
	GetTenantByID(ctx context.Context, tenantID uuid.UUID) (*domain.Tenant, error)
	GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	UpdateTenant(ctx context.Context, tenant *domain.Tenant) error
	DeleteTenant(ctx context.Context, tenantID uuid.UUID) error
	ListTenants(ctx context.Context, limit, offset int) ([]*domain.Tenant, error)
}

// TenantRepositoryPort defines tenant data access interface
type TenantRepositoryPort interface {
	Create(ctx context.Context, tenant *domain.Tenant) error
	GetByID(ctx context.Context, tenantID uuid.UUID) (*domain.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	Update(ctx context.Context, tenant *domain.Tenant) error
	Delete(ctx context.Context, tenantID uuid.UUID) error
	List(ctx context.Context, limit, offset int) ([]*domain.Tenant, error)
	Search(ctx context.Context, query string, limit, offset int) ([]*domain.Tenant, error)
}
