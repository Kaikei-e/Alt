package scraping_domain_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/port/scraping_domain_port"
	"context"

	"github.com/google/uuid"
)

// ScrapingDomainGateway implements ScrapingDomainPort
type ScrapingDomainGateway struct {
	altDB *alt_db.AltDBRepository
}

// NewScrapingDomainGateway creates a new ScrapingDomainGateway
func NewScrapingDomainGateway(altDB *alt_db.AltDBRepository) scraping_domain_port.ScrapingDomainPort {
	return &ScrapingDomainGateway{altDB: altDB}
}

// GetByDomain retrieves a scraping domain by domain name
func (g *ScrapingDomainGateway) GetByDomain(ctx context.Context, domainName string) (*domain.ScrapingDomain, error) {
	if g.altDB == nil {
		return nil, nil
	}
	return g.altDB.GetScrapingDomainByDomain(ctx, domainName)
}

// GetByID retrieves a scraping domain by ID
func (g *ScrapingDomainGateway) GetByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error) {
	if g.altDB == nil {
		return nil, nil
	}
	return g.altDB.GetScrapingDomainByID(ctx, id)
}

// Save saves or updates a scraping domain
func (g *ScrapingDomainGateway) Save(ctx context.Context, sd *domain.ScrapingDomain) error {
	if g.altDB == nil {
		return nil
	}
	return g.altDB.SaveScrapingDomain(ctx, sd)
}

// List lists scraping domains with pagination
func (g *ScrapingDomainGateway) List(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error) {
	if g.altDB == nil {
		return []*domain.ScrapingDomain{}, nil
	}
	return g.altDB.ListScrapingDomains(ctx, offset, limit)
}

// UpdatePolicy updates only the policy fields of a scraping domain
func (g *ScrapingDomainGateway) UpdatePolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error {
	if g.altDB == nil {
		return nil
	}
	return g.altDB.UpdateScrapingDomainPolicy(ctx, id, update)
}
