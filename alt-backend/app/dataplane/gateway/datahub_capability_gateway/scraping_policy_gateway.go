package datahub_capability_gateway

import (
	"context"
	"fmt"

	"alt/domain"
	"alt/shared/driver/alt_db"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// §2.L Scraping policy
// ---------------------------------------------------------------------------

type scrapingDriver interface {
	GetScrapingDomainByDomain(ctx context.Context, domainName string) (*domain.ScrapingDomain, error)
	GetScrapingDomainByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error)
	SaveScrapingDomain(ctx context.Context, sd *domain.ScrapingDomain) error
	ListScrapingDomains(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error)
	UpdateScrapingDomainPolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error
	SaveDeclinedDomain(ctx context.Context, userID, domainName string) error
	IsDomainDeclined(ctx context.Context, userID, domainName string) (bool, error)
}

// ScrapingPolicyGateway implements datahub_capability_port.ScrapingPolicyPort.
type ScrapingPolicyGateway struct {
	db scrapingDriver
}

func NewScrapingPolicyGateway(db *alt_db.AltDBRepository) *ScrapingPolicyGateway {
	return &ScrapingPolicyGateway{db: db}
}

func (g *ScrapingPolicyGateway) GetByDomain(ctx context.Context, domainName string) (*domain.ScrapingDomain, error) {
	sd, err := g.db.GetScrapingDomainByDomain(ctx, domainName)
	if err != nil {
		return nil, fmt.Errorf("get scraping domain %q: %w", domainName, err)
	}
	return sd, nil
}

func (g *ScrapingPolicyGateway) GetByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error) {
	sd, err := g.db.GetScrapingDomainByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get scraping domain %s: %w", id, err)
	}
	return sd, nil
}

// Save turns the driver's in-place mutation into a return value.
//
// SaveScrapingDomain assigns the id, created_at and updated_at by writing into
// the struct it was handed. That worked when the caller shared an address
// space with the query; over an RPC the caller's struct is a different object
// on a different host, so the assigned values have to travel back explicitly.
func (g *ScrapingPolicyGateway) Save(ctx context.Context, sd *domain.ScrapingDomain) (*domain.ScrapingDomain, error) {
	if sd == nil {
		return nil, fmt.Errorf("save scraping domain: nil domain")
	}
	saved := *sd
	if err := g.db.SaveScrapingDomain(ctx, &saved); err != nil {
		return nil, fmt.Errorf("save scraping domain %q: %w", sd.Domain, err)
	}
	return &saved, nil
}

func (g *ScrapingPolicyGateway) List(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error) {
	domains, err := g.db.ListScrapingDomains(ctx, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("list scraping domains: %w", err)
	}
	return domains, nil
}

func (g *ScrapingPolicyGateway) UpdatePolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error {
	if err := g.db.UpdateScrapingDomainPolicy(ctx, id, update); err != nil {
		return fmt.Errorf("update scraping domain policy %s: %w", id, err)
	}
	return nil
}

func (g *ScrapingPolicyGateway) SaveDeclinedDomain(ctx context.Context, userID, domainName string) error {
	if err := g.db.SaveDeclinedDomain(ctx, userID, domainName); err != nil {
		return fmt.Errorf("save declined domain %q: %w", domainName, err)
	}
	return nil
}

func (g *ScrapingPolicyGateway) IsDomainDeclined(ctx context.Context, userID, domainName string) (bool, error) {
	declined, err := g.db.IsDomainDeclined(ctx, userID, domainName)
	if err != nil {
		return false, fmt.Errorf("check declined domain %q: %w", domainName, err)
	}
	return declined, nil
}
