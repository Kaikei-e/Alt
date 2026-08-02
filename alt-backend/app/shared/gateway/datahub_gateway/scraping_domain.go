package datahub_gateway

import (
	"context"
	"fmt"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// ScrapingDomainGateway is the recorded scraping policy per publisher
// (catalog §2.L).
//
// alt-harvester's daily job fetches robots.txt over HTTP — external I/O, so it
// stays there (ADR-000954 D4) — parses it, and writes the result through
// Save/UpdatePolicy. alt-backend reads the same rows on the article fetch path
// to decide whether it may retrieve a body at all.
type ScrapingDomainGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewScrapingDomainGateway(client datahubv1connect.DataHubServiceClient) *ScrapingDomainGateway {
	if client == nil {
		panic("datahub_gateway: ScrapingDomainGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &ScrapingDomainGateway{client: client}
}

// GetByDomain returns the recorded policy for a hostname, or (nil, nil) when
// none exists.
//
// The nil-without-error shape is what the compliance check on the article
// fetch path reads: "no policy recorded" makes it fall back to a live
// robots.txt fetch, whereas an error aborts the fetch. Collapsing the two
// would turn a first-time domain into a failure.
func (g *ScrapingDomainGateway) GetByDomain(ctx context.Context, domainName string) (*domain.ScrapingDomain, error) {
	resp, err := g.client.GetScrapingDomainByDomain(ctx, connect.NewRequest(&datahubv1.GetScrapingDomainByDomainRequest{
		Domain: domainName,
	}))
	if err != nil {
		return nil, fmt.Errorf("get scraping domain %q: %w", domainName, err)
	}
	sd, err := scrapingDomainFromProto(resp.Msg.GetScrapingDomain())
	if err != nil {
		return nil, fmt.Errorf("get scraping domain %q: %w", domainName, err)
	}
	return sd, nil
}

// GetByID returns the recorded policy by row id, or (nil, nil).
func (g *ScrapingDomainGateway) GetByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error) {
	resp, err := g.client.GetScrapingDomainByID(ctx, connect.NewRequest(&datahubv1.GetScrapingDomainByIDRequest{
		Id: id.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("get scraping domain %s: %w", id, err)
	}
	sd, err := scrapingDomainFromProto(resp.Msg.GetScrapingDomain())
	if err != nil {
		return nil, fmt.Errorf("get scraping domain %s: %w", id, err)
	}
	return sd, nil
}

// Save upserts a scraping domain by hostname.
//
// It writes the assigned id and timestamps back into the caller's struct. The
// driver this replaces did the same by mutating its argument in place, and the
// robots refresh job relies on it: it saves a freshly discovered domain and
// then updates that domain's policy by id. Across a process boundary the
// provider is the only side that knows the assigned values, so they come back
// in the response and are copied here rather than being invented locally.
func (g *ScrapingDomainGateway) Save(ctx context.Context, sd *domain.ScrapingDomain) error {
	if sd == nil {
		return fmt.Errorf("save scraping domain: nil domain")
	}

	resp, err := g.client.SaveScrapingDomain(ctx, connect.NewRequest(&datahubv1.SaveScrapingDomainRequest{
		ScrapingDomain: scrapingDomainToProto(sd),
	}))
	if err != nil {
		return fmt.Errorf("save scraping domain %q: %w", sd.Domain, err)
	}

	saved, err := scrapingDomainFromProto(resp.Msg.GetScrapingDomain())
	if err != nil {
		return fmt.Errorf("save scraping domain %q: %w", sd.Domain, err)
	}
	if saved != nil {
		*sd = *saved
	}
	return nil
}

// List pages through the recorded policies.
func (g *ScrapingDomainGateway) List(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error) {
	resp, err := g.client.ListScrapingDomains(ctx, connect.NewRequest(&datahubv1.ListScrapingDomainsRequest{
		Offset: safeconv.Int32(offset),
		Limit:  safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("list scraping domains (offset %d, limit %d): %w", offset, limit, err)
	}

	domains := make([]*domain.ScrapingDomain, 0, len(resp.Msg.GetScrapingDomains()))
	for _, raw := range resp.Msg.GetScrapingDomains() {
		sd, err := scrapingDomainFromProto(raw)
		if err != nil {
			return nil, fmt.Errorf("list scraping domains: %w", err)
		}
		if sd != nil {
			domains = append(domains, sd)
		}
	}
	return domains, nil
}

// UpdatePolicy applies a partial policy update. An unknown id is an error,
// not a silent zero-row update.
func (g *ScrapingDomainGateway) UpdatePolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error {
	_, err := g.client.UpdateScrapingDomainPolicy(ctx, connect.NewRequest(&datahubv1.UpdateScrapingDomainPolicyRequest{
		Id:     id.String(),
		Update: scrapingPolicyUpdateToProto(update),
	}))
	if err != nil {
		return fmt.Errorf("update scraping domain policy %s: %w", id, err)
	}
	return nil
}

// DeclinedDomainGateway records and reads a user's refusal to have a domain
// fetched in full (catalog §2.L, W3-L6 / W3-L7).
//
// Separate from ScrapingDomainGateway because it answers a different question
// about a different table: scraping_domains is what the publisher permits,
// declined_domains is what this user asked us not to do.
type DeclinedDomainGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewDeclinedDomainGateway(client datahubv1connect.DataHubServiceClient) *DeclinedDomainGateway {
	if client == nil {
		panic("datahub_gateway: DeclinedDomainGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &DeclinedDomainGateway{client: client}
}

// SaveDeclinedDomain records the refusal. Idempotent.
func (g *DeclinedDomainGateway) SaveDeclinedDomain(ctx context.Context, userID, domainName string) error {
	_, err := g.client.SaveDeclinedDomain(ctx, connect.NewRequest(&datahubv1.SaveDeclinedDomainRequest{
		UserId: userID,
		Domain: domainName,
	}))
	if err != nil {
		return fmt.Errorf("save declined domain %q: %w", domainName, err)
	}
	return nil
}

// IsDomainDeclined reports whether the user refused this domain.
func (g *DeclinedDomainGateway) IsDomainDeclined(ctx context.Context, userID, domainName string) (bool, error) {
	resp, err := g.client.IsDomainDeclined(ctx, connect.NewRequest(&datahubv1.IsDomainDeclinedRequest{
		UserId: userID,
		Domain: domainName,
	}))
	if err != nil {
		return false, fmt.Errorf("check declined domain %q: %w", domainName, err)
	}
	return resp.Msg.GetDeclined(), nil
}
