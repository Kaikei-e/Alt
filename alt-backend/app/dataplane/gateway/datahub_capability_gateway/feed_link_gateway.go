package datahub_capability_gateway

import (
	"context"
	"fmt"

	"alt/domain"
	"alt/shared/driver/alt_db"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// §2.F Feed links
// ---------------------------------------------------------------------------

type feedLinkDriver interface {
	RegisterRSSFeedLink(ctx context.Context, link string) error
	FetchFeedLinkIDByURL(ctx context.Context, feedURL string) (*string, error)
	FetchFeedLinks(ctx context.Context) ([]*domain.FeedLink, error)
	FetchFeedLinksWithAvailability(ctx context.Context) ([]*domain.FeedLinkWithHealth, error)
	DeleteFeedLink(ctx context.Context, id uuid.UUID) error
	ListFeedLinkDomains(ctx context.Context) ([]domain.FeedLinkDomain, error)
	FetchRSSFeedURLs(ctx context.Context) ([]domain.FeedLink, error)
	FetchFeedLinksForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error)
}

// FeedLinkGateway implements datahub_capability_port.FeedLinkPort.
type FeedLinkGateway struct {
	db feedLinkDriver
}

func NewFeedLinkGateway(db *alt_db.AltDBRepository) *FeedLinkGateway {
	return &FeedLinkGateway{db: db}
}

// Register reports whether the URL was already subscribed.
//
// The driver swallows SQLSTATE 23505 and answers success, which is the right
// behaviour — registering twice is not an error — but it made "added" and
// "already had it" indistinguishable, so the OPML import re-queried to tell
// them apart. Resolving the id first turns that extra query into information
// the caller can use, and keeps the duplicate branch as the safety net it was
// rather than the mechanism.
func (g *FeedLinkGateway) Register(ctx context.Context, url string) (bool, error) {
	existingID, err := g.db.FetchFeedLinkIDByURL(ctx, url)
	if err != nil {
		return false, fmt.Errorf("resolve feed link %q: %w", url, err)
	}
	if existingID != nil {
		return true, nil
	}

	if err := g.db.RegisterRSSFeedLink(ctx, url); err != nil {
		return false, fmt.Errorf("register feed link %q: %w", url, err)
	}
	return false, nil
}

// BulkRegister subscribes to many URLs and reports per-URL outcomes.
//
// Deliberately not one transaction. A 500-entry OPML file with one bad outline
// must import the other 499; rolling the batch back would make the whole
// import hostage to its worst row, and the caller has no way to find that row
// except by bisecting the file.
func (g *FeedLinkGateway) BulkRegister(ctx context.Context, urls []string) (int, int, []string, error) {
	var registered, skipped int
	failed := make([]string, 0)

	for _, url := range urls {
		alreadyExisted, err := g.Register(ctx, url)
		switch {
		case err != nil:
			failed = append(failed, url)
		case alreadyExisted:
			skipped++
		default:
			registered++
		}
	}
	return registered, skipped, failed, nil
}

func (g *FeedLinkGateway) List(ctx context.Context) ([]*domain.FeedLink, error) {
	links, err := g.db.FetchFeedLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feed links: %w", err)
	}
	return links, nil
}

func (g *FeedLinkGateway) ListWithHealth(ctx context.Context) ([]*domain.FeedLinkWithHealth, error) {
	links, err := g.db.FetchFeedLinksWithAvailability(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feed links with health: %w", err)
	}
	return links, nil
}

func (g *FeedLinkGateway) Delete(ctx context.Context, id uuid.UUID) error {
	if err := g.db.DeleteFeedLink(ctx, id); err != nil {
		return fmt.Errorf("delete feed link %s: %w", id, err)
	}
	return nil
}

func (g *FeedLinkGateway) ResolveIDByURL(ctx context.Context, feedURL string) (*string, error) {
	id, err := g.db.FetchFeedLinkIDByURL(ctx, feedURL)
	if err != nil {
		return nil, fmt.Errorf("resolve feed link id for %q: %w", feedURL, err)
	}
	return id, nil
}

func (g *FeedLinkGateway) ListDomains(ctx context.Context) ([]domain.FeedLinkDomain, error) {
	domains, err := g.db.ListFeedLinkDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feed link domains: %w", err)
	}
	return domains, nil
}

func (g *FeedLinkGateway) ListPollable(ctx context.Context) ([]domain.FeedLink, error) {
	links, err := g.db.FetchRSSFeedURLs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pollable feed links: %w", err)
	}
	return links, nil
}

func (g *FeedLinkGateway) ListForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error) {
	entries, err := g.db.FetchFeedLinksForExport(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feed links for export: %w", err)
	}
	return entries, nil
}

// ---------------------------------------------------------------------------
// §2.G Feed link availability
// ---------------------------------------------------------------------------

type feedLinkAvailabilityDriver interface {
	RecordFeedLinkFailure(ctx context.Context, feedURL, reason string, disableAfter int) (*domain.FeedLinkAvailability, bool, error)
	ResetFeedLinkFailures(ctx context.Context, feedURL string) error
}

// FeedLinkAvailabilityGateway implements
// datahub_capability_port.FeedLinkAvailabilityPort.
type FeedLinkAvailabilityGateway struct {
	db feedLinkAvailabilityDriver
}

func NewFeedLinkAvailabilityGateway(db *alt_db.AltDBRepository) *FeedLinkAvailabilityGateway {
	return &FeedLinkAvailabilityGateway{db: db}
}

func (g *FeedLinkAvailabilityGateway) RecordFailure(ctx context.Context, feedURL, reason string, disableAfter int) (*domain.FeedLinkAvailability, bool, error) {
	availability, disabledNow, err := g.db.RecordFeedLinkFailure(ctx, feedURL, reason, disableAfter)
	if err != nil {
		return nil, false, fmt.Errorf("record feed link failure for %q: %w", feedURL, err)
	}
	return availability, disabledNow, nil
}

func (g *FeedLinkAvailabilityGateway) ResetFailures(ctx context.Context, feedURL string) error {
	if err := g.db.ResetFeedLinkFailures(ctx, feedURL); err != nil {
		return fmt.Errorf("reset feed link failures for %q: %w", feedURL, err)
	}
	return nil
}
