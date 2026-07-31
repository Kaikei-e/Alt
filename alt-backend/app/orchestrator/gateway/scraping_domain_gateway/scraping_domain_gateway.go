package scraping_domain_gateway

import (
	"alt/domain"
	"alt/orchestrator/port/scraping_domain_port"
	"alt/shared/driver/alt_db"
	"context"

	"github.com/google/uuid"
)

// ScrapingDomainGateway implements ScrapingDomainPort.
//
// UNREFERENCED since ADR-000954 Wave 3 batch 1. scraping_domains moved to
// alt-data-hub (catalog §2.L), so both composition roots wire
// datahub_gateway.ScrapingDomainGateway against the same port and this
// constructor has no caller. Removed with the rest of the direct alt_db
// surface in batch 6 — see the note on image_proxy_gateway.CacheGateway.
//
// Its `if g.altDB == nil { return nil, nil }` guards are also why the
// replacement panics on a nil client instead: a gateway that answers "no
// policy recorded" when it has no database is indistinguishable from one
// looking at an empty table, and on this particular path that answer decides
// whether a publisher's robots.txt is honoured.
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
