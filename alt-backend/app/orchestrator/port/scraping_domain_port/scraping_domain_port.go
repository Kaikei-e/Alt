package scraping_domain_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -source=scraping_domain_port.go -destination=../../mocks/mock_scraping_domain_port.go

// ScrapingDomainPort defines the interface for scraping domain operations
type ScrapingDomainPort interface {
	// GetByDomain retrieves a scraping domain by domain name
	GetByDomain(ctx context.Context, domain string) (*domain.ScrapingDomain, error)
	// GetByID retrieves a scraping domain by ID
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error)
	// Save saves or updates a scraping domain
	Save(ctx context.Context, domain *domain.ScrapingDomain) error
	// List lists scraping domains with pagination
	List(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error)
	// UpdatePolicy updates only the policy fields of a scraping domain
	UpdatePolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error
}
