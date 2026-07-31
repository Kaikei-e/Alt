// Feed link and feed capabilities (ADR-000954 Wave 3 batch 3, capability
// catalog §2.F / §2.G / §2.H).
//
// Same shape as the rest of this package: thin adapters over the alt_db
// drivers, which already hold the transaction boundaries and the ON CONFLICT
// clauses these capabilities are drawn around. What is added here is the shape
// the port asks for — domain rows instead of driver models, a scope enum
// instead of four near-identical methods, and a returned outcome instead of a
// second query to find out what happened.
package datahub_capability_gateway

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	"alt/orchestrator/driver/models"
	"alt/shared/driver/alt_db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// ---------------------------------------------------------------------------
// §2.H Feeds
// ---------------------------------------------------------------------------

type feedDriver interface {
	RegisterMultipleFeedsWithState(ctx context.Context, feeds []models.Feed) ([]alt_db.FeedRegistrationResult, error)
	FetchAllFeedsListCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error)
	FetchUnreadFeedsListCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error)
	FetchReadFeedsListCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]*models.Feed, error)
	FetchFavoriteFeedsListCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]*models.Feed, error)
	FetchUnreadFeedsListPageForUser(ctx context.Context, userID uuid.UUID, page int) ([]*models.Feed, error)
	FetchFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error)
	FetchFeedsList(ctx context.Context) ([]*models.Feed, error)
	FetchFeedsListLimit(ctx context.Context, limit int) ([]*models.Feed, error)
	GetSingleFeed(ctx context.Context) (*models.Feed, error)
	FetchFeedsByFeedLinkID(ctx context.Context, feedLinkID uuid.UUID) ([]*alt_db.FeedPageRow, error)
	FetchFeedSummaryForUser(ctx context.Context, feedURL *url.URL, userID *uuid.UUID) (*domain.FeedSummary, error)
	FetchArticleSummaryByArticleIDForUser(ctx context.Context, articleID string, userID *uuid.UUID) (*domain.FeedSummary, error)
	SearchFeedsByTitle(ctx context.Context, query string, userID string) ([]*domain.FeedItem, error)
	FetchRandomFeed(ctx context.Context) (*domain.Feed, error)
	GetFeedURLsByArticleIDs(ctx context.Context, articleIDs []string) ([]domain.FeedAndArticle, error)
	FetchFeedTitlesByIDs(ctx context.Context, feedIDs []uuid.UUID) (map[uuid.UUID]string, error)
	FetchInoreaderSummariesByURLs(ctx context.Context, urls []string) ([]*models.InoreaderSummary, error)
}

// FeedGateway implements datahub_capability_port.FeedPort.
type FeedGateway struct {
	db feedDriver
}

func NewFeedGateway(db *alt_db.AltDBRepository) *FeedGateway {
	return &FeedGateway{db: db}
}

func (g *FeedGateway) Register(ctx context.Context, feeds []domain.FeedRegistration) ([]domain.FeedRegistrationResult, error) {
	if len(feeds) == 0 {
		return []domain.FeedRegistrationResult{}, nil
	}

	items := make([]models.Feed, 0, len(feeds))
	for _, f := range feeds {
		items = append(items, models.Feed{
			Title:       f.Title,
			Description: f.Description,
			WebsiteURL:  f.WebsiteURL,
			PubDate:     f.PubDate,
			CreatedAt:   f.CreatedAt,
			UpdatedAt:   f.UpdatedAt,
			FeedLinkID:  f.FeedLinkID,
			OgImageURL:  f.OgImageURL,
		})
	}

	rows, err := g.db.RegisterMultipleFeedsWithState(ctx, items)
	if err != nil {
		return nil, fmt.Errorf("register %d feeds: %w", len(feeds), err)
	}

	results := make([]domain.FeedRegistrationResult, 0, len(rows))
	for _, r := range rows {
		results = append(results, domain.FeedRegistrationResult{FeedID: r.FeedID, Created: r.Created})
	}
	return results, nil
}

func (g *FeedGateway) ListCursor(ctx context.Context, scope datahub_capability_port.FeedScope, userID uuid.UUID, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*domain.FeedRow, error) {
	var (
		rows []*models.Feed
		err  error
	)
	switch scope {
	case datahub_capability_port.FeedScopeAll:
		rows, err = g.db.FetchAllFeedsListCursorForUser(ctx, userID, cursor, limit, excludeFeedLinkIDs)
	case datahub_capability_port.FeedScopeUnread:
		rows, err = g.db.FetchUnreadFeedsListCursorForUser(ctx, userID, cursor, limit, excludeFeedLinkIDs)
	case datahub_capability_port.FeedScopeRead:
		// Read and favourite walks page by read_at / favourited_at, not by
		// feeds.created_at, so they take no exclusion list: the client-side
		// "hide this source" filter only exists on the two timeline scopes.
		rows, err = g.db.FetchReadFeedsListCursorForUser(ctx, userID, cursor, limit)
	case datahub_capability_port.FeedScopeFavorite:
		rows, err = g.db.FetchFavoriteFeedsListCursorForUser(ctx, userID, cursor, limit)
	default:
		return nil, fmt.Errorf("list feeds cursor: unknown scope %d", scope)
	}
	if err != nil {
		return nil, fmt.Errorf("list feeds cursor (scope %d): %w", scope, err)
	}
	return feedRowsFromModels(rows), nil
}

func (g *FeedGateway) ListPage(ctx context.Context, page int, unreadOnly bool, userID uuid.UUID) ([]*domain.FeedRow, error) {
	var (
		rows []*models.Feed
		err  error
	)
	if unreadOnly {
		rows, err = g.db.FetchUnreadFeedsListPageForUser(ctx, userID, page)
	} else {
		rows, err = g.db.FetchFeedsListPage(ctx, page)
	}
	if err != nil {
		return nil, fmt.Errorf("list feeds page %d: %w", page, err)
	}
	return feedRowsFromModels(rows), nil
}

// ListLimit treats a non-positive limit as the unbounded list, which the
// driver caps at 10000. That ceiling is the existing behaviour of
// FetchFeedsList, not "no limit", and saying so here keeps a caller from
// believing zero means everything.
func (g *FeedGateway) ListLimit(ctx context.Context, limit int) ([]*domain.FeedRow, error) {
	var (
		rows []*models.Feed
		err  error
	)
	if limit <= 0 {
		rows, err = g.db.FetchFeedsList(ctx)
	} else {
		rows, err = g.db.FetchFeedsListLimit(ctx, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list feeds (limit %d): %w", limit, err)
	}
	return feedRowsFromModels(rows), nil
}

// GetSingle maps "no feeds at all" to nil without error.
//
// The driver reports it as an error because in-process the only caller treated
// an empty table as a failure. Over an RPC the distinction matters: a fresh
// install has no feeds, and answering Internal for that would make an empty
// database look like a broken one.
func (g *FeedGateway) GetSingle(ctx context.Context) (*domain.FeedRow, error) {
	row, err := g.db.GetSingleFeed(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get single feed: %w", err)
	}
	return feedRowFromModel(row), nil
}

func (g *FeedGateway) ListByFeedLinkID(ctx context.Context, feedLinkID uuid.UUID) ([]*domain.FeedRow, error) {
	rows, err := g.db.FetchFeedsByFeedLinkID(ctx, feedLinkID)
	if err != nil {
		return nil, fmt.Errorf("list feeds for feed link %s: %w", feedLinkID, err)
	}

	out := make([]*domain.FeedRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, &domain.FeedRow{
			ID:          r.FeedID.String(),
			Title:       r.Title,
			Description: r.Description,
			WebsiteURL:  r.Link,
			PubDate:     r.PubDate,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
			ArticleID:   r.ArticleID,
			OgImageURL:  r.OgImageURL,
		})
	}
	return out, nil
}

// GetSummary maps "no summary yet" to nil without error.
//
// The driver returns pgx.ErrNoRows for it, which the in-process caller read as
// "generate one". Over an RPC that has to be an unset field rather than an
// error, or the summarise path would treat every unsummarised article as a
// data plane fault.
func (g *FeedGateway) GetSummary(ctx context.Context, feedURL string, userID *uuid.UUID) (*domain.FeedSummary, error) {
	parsed, err := url.Parse(feedURL)
	if err != nil {
		return nil, fmt.Errorf("parse feed url %q: %w", feedURL, err)
	}

	summary, err := g.db.FetchFeedSummaryForUser(ctx, parsed, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get feed summary for %q: %w", feedURL, err)
	}
	return summary, nil
}

func (g *FeedGateway) GetSummaryByArticleID(ctx context.Context, articleID string, userID *uuid.UUID) (*domain.FeedSummary, error) {
	summary, err := g.db.FetchArticleSummaryByArticleIDForUser(ctx, articleID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get summary for article %s: %w", articleID, err)
	}
	return summary, nil
}

// SearchByTitle re-expresses the driver's domain.FeedItem results as rows.
//
// The driver builds a FeedItem because its only caller renders one, and part
// of that build is choosing created_at when pub_date is NULL. Preserving that
// choice here rather than sending a NULL onward keeps the search results
// ordering identical to what the SQL produced.
func (g *FeedGateway) SearchByTitle(ctx context.Context, query, userID string) ([]*domain.FeedRow, error) {
	items, err := g.db.SearchFeedsByTitle(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("search feeds by title %q: %w", query, err)
	}

	out := make([]*domain.FeedRow, 0, len(items))
	for _, item := range items {
		out = append(out, &domain.FeedRow{
			Title:       item.Title,
			Description: item.Description,
			WebsiteURL:  item.Link,
			PubDate:     item.PublishedParsed,
			CreatedAt:   item.PublishedParsed,
		})
	}
	return out, nil
}

func (g *FeedGateway) GetRandom(ctx context.Context) (*domain.Feed, error) {
	feed, err := g.db.FetchRandomFeed(ctx)
	if err != nil {
		return nil, fmt.Errorf("get random feed: %w", err)
	}
	return feed, nil
}

func (g *FeedGateway) GetFeedURLsByArticleIDs(ctx context.Context, articleIDs []string) ([]domain.FeedAndArticle, error) {
	pairs, err := g.db.GetFeedURLsByArticleIDs(ctx, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("get feed urls for %d articles: %w", len(articleIDs), err)
	}
	return pairs, nil
}

func (g *FeedGateway) BatchGetTitlesByIDs(ctx context.Context, feedIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	titles, err := g.db.FetchFeedTitlesByIDs(ctx, feedIDs)
	if err != nil {
		return nil, fmt.Errorf("batch get feed titles (%d): %w", len(feedIDs), err)
	}
	return titles, nil
}

func (g *FeedGateway) GetInoreaderSummariesByURLs(ctx context.Context, urls []string) ([]*domain.InoreaderSummary, error) {
	rows, err := g.db.FetchInoreaderSummariesByURLs(ctx, urls)
	if err != nil {
		return nil, fmt.Errorf("get inoreader summaries (%d urls): %w", len(urls), err)
	}

	out := make([]*domain.InoreaderSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, &domain.InoreaderSummary{
			ArticleURL:  r.ArticleURL,
			Title:       r.Title,
			Author:      r.Author,
			Content:     r.Content,
			ContentType: r.ContentType,
			PublishedAt: r.PublishedAt,
			FetchedAt:   r.FetchedAt,
			InoreaderID: r.InoreaderID,
		})
	}
	return out, nil
}

func feedRowsFromModels(rows []*models.Feed) []*domain.FeedRow {
	out := make([]*domain.FeedRow, 0, len(rows))
	for _, r := range rows {
		if row := feedRowFromModel(r); row != nil {
			out = append(out, row)
		}
	}
	return out
}

func feedRowFromModel(f *models.Feed) *domain.FeedRow {
	if f == nil {
		return nil
	}
	return &domain.FeedRow{
		ID:          f.ID,
		Title:       f.Title,
		Description: f.Description,
		WebsiteURL:  f.WebsiteURL,
		PubDate:     f.PubDate,
		CreatedAt:   f.CreatedAt,
		UpdatedAt:   f.UpdatedAt,
		ArticleID:   f.ArticleID,
		IsRead:      f.IsRead,
		FeedLinkID:  f.FeedLinkID,
		OgImageURL:  f.OgImageURL,
	}
}
