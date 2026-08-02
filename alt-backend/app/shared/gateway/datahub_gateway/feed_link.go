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

// FeedLinkGateway is the subscription list — the URLs the collector polls
// (catalog §2.F).
//
// Method names match the alt_db driver methods they replace, so migrating a
// caller is a DI change and nothing else. The one exception is
// RegisterFeedLinkBulk, which is named for the port it satisfies rather than
// for a driver method: there was never a bulk driver, only a loop in
// opml_gateway, and the loop is what this replaces.
type FeedLinkGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewFeedLinkGateway(client datahubv1connect.DataHubServiceClient) *FeedLinkGateway {
	if client == nil {
		panic("datahub_gateway: FeedLinkGateway requires a DataHubService client — " +
			"a nil client would make every feed registration and every collector tick " +
			"fail identically to an empty subscription list (see .claude/rules/di-wiring.md)")
	}
	return &FeedLinkGateway{client: client}
}

// RegisterRSSFeedLink subscribes to a URL, or does nothing if it is already
// subscribed.
//
// The URL arrives sanitised: tracking-parameter stripping and the SSRF check
// are pure functions and stay on this side of the boundary (ADR-000954 D4).
func (g *FeedLinkGateway) RegisterRSSFeedLink(ctx context.Context, link string) error {
	_, err := g.client.RegisterFeedLink(ctx, connect.NewRequest(&datahubv1.RegisterFeedLinkRequest{
		Url: link,
	}))
	if err != nil {
		return fmt.Errorf("register feed link %q: %w", link, err)
	}
	return nil
}

// RegisterFeedLinkBulk imports many subscriptions in one call.
//
// Its predecessor looped in the gateway, issuing an exists-check and an insert
// per URL. In-process that was two queries; across a process boundary it would
// be two round trips per outline, so a 500-entry OPML file would cost a
// thousand. The loop moves to the provider and the call becomes one.
//
// Validation stays here. IsAllowedURL and StripTrackingParams are pure
// functions over a string, and the count they produce — how many outlines were
// rejected before the data plane ever saw them — is part of the result the
// user is shown.
func (g *FeedLinkGateway) RegisterFeedLinkBulk(ctx context.Context, urls []string) (*domain.OPMLImportResult, error) {
	result := &domain.OPMLImportResult{Total: len(urls)}
	if len(urls) == 0 {
		return result, nil
	}

	resp, err := g.client.BulkRegisterFeedLinks(ctx, connect.NewRequest(&datahubv1.BulkRegisterFeedLinksRequest{
		Urls: urls,
	}))
	if err != nil {
		return nil, fmt.Errorf("bulk register %d feed links: %w", len(urls), err)
	}

	result.Imported = int(resp.Msg.GetRegistered())
	result.Skipped = int(resp.Msg.GetSkipped())
	result.Failed = len(resp.Msg.GetFailedUrls())
	result.FailedURLs = resp.Msg.GetFailedUrls()
	return result, nil
}

// FetchFeedLinkIDByURL returns nil without error for a URL nobody subscribes
// to.
//
// The nil-without-error shape is load-bearing and inherited: the registration
// flow reads it as "this is new" and continues. An error there would make
// every first-time registration look like a database fault.
func (g *FeedLinkGateway) FetchFeedLinkIDByURL(ctx context.Context, feedURL string) (*string, error) {
	resp, err := g.client.ResolveFeedLinkIDByURL(ctx, connect.NewRequest(&datahubv1.ResolveFeedLinkIDByURLRequest{
		FeedUrl: feedURL,
	}))
	if err != nil {
		return nil, fmt.Errorf("resolve feed link id for %q: %w", feedURL, err)
	}
	if resp.Msg.FeedLinkId == nil {
		return nil, nil
	}
	id := resp.Msg.GetFeedLinkId()
	return &id, nil
}

func (g *FeedLinkGateway) FetchFeedLinks(ctx context.Context) ([]*domain.FeedLink, error) {
	resp, err := g.client.ListFeedLinks(ctx, connect.NewRequest(&datahubv1.ListFeedLinksRequest{}))
	if err != nil {
		return nil, fmt.Errorf("list feed links: %w", err)
	}

	links := make([]*domain.FeedLink, 0, len(resp.Msg.GetFeedLinks()))
	for _, l := range resp.Msg.GetFeedLinks() {
		link, convErr := feedLinkFromProto(l)
		if convErr != nil {
			return nil, convErr
		}
		links = append(links, link)
	}
	return links, nil
}

func (g *FeedLinkGateway) FetchFeedLinksWithAvailability(ctx context.Context) ([]*domain.FeedLinkWithHealth, error) {
	resp, err := g.client.ListFeedLinksWithHealth(ctx, connect.NewRequest(&datahubv1.ListFeedLinksWithHealthRequest{}))
	if err != nil {
		return nil, fmt.Errorf("list feed links with health: %w", err)
	}

	out := make([]*domain.FeedLinkWithHealth, 0, len(resp.Msg.GetFeedLinks()))
	for _, l := range resp.Msg.GetFeedLinks() {
		link, convErr := feedLinkFromProto(l.GetFeedLink())
		if convErr != nil {
			return nil, convErr
		}
		health := &domain.FeedLinkWithHealth{FeedLink: *link}
		// An absent availability message stays a nil pointer, which
		// GetHealthStatus reports as "unknown". Substituting a zero-valued
		// struct would report a never-polled link as healthy.
		if a := l.GetAvailability(); a != nil {
			availability, convErr := feedLinkAvailabilityFromProto(a)
			if convErr != nil {
				return nil, convErr
			}
			health.Availability = availability
		}
		out = append(out, health)
	}
	return out, nil
}

func (g *FeedLinkGateway) DeleteFeedLink(ctx context.Context, id uuid.UUID) error {
	_, err := g.client.DeleteFeedLink(ctx, connect.NewRequest(&datahubv1.DeleteFeedLinkRequest{
		Id: id.String(),
	}))
	if err != nil {
		return fmt.Errorf("delete feed link %s: %w", id, err)
	}
	return nil
}

func (g *FeedLinkGateway) ListFeedLinkDomains(ctx context.Context) ([]domain.FeedLinkDomain, error) {
	resp, err := g.client.ListFeedLinkDomains(ctx, connect.NewRequest(&datahubv1.ListFeedLinkDomainsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("list feed link domains: %w", err)
	}

	domains := make([]domain.FeedLinkDomain, 0, len(resp.Msg.GetDomains()))
	for _, d := range resp.Msg.GetDomains() {
		domains = append(domains, domain.FeedLinkDomain{Domain: d.GetDomain(), Scheme: d.GetScheme()})
	}
	return domains, nil
}

// FetchRSSFeedURLs returns the links worth polling: active, or never assessed.
func (g *FeedLinkGateway) FetchRSSFeedURLs(ctx context.Context) ([]domain.FeedLink, error) {
	resp, err := g.client.ListRSSFeedURLs(ctx, connect.NewRequest(&datahubv1.ListRSSFeedURLsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("list rss feed urls: %w", err)
	}

	links := make([]domain.FeedLink, 0, len(resp.Msg.GetFeedLinks()))
	for _, l := range resp.Msg.GetFeedLinks() {
		link, convErr := feedLinkFromProto(l)
		if convErr != nil {
			return nil, convErr
		}
		links = append(links, *link)
	}
	return links, nil
}

func (g *FeedLinkGateway) FetchFeedLinksForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error) {
	resp, err := g.client.ListFeedLinksForExport(ctx, connect.NewRequest(&datahubv1.ListFeedLinksForExportRequest{}))
	if err != nil {
		return nil, fmt.Errorf("list feed links for export: %w", err)
	}

	entries := make([]*domain.FeedLinkForExport, 0, len(resp.Msg.GetEntries()))
	for _, e := range resp.Msg.GetEntries() {
		entries = append(entries, &domain.FeedLinkForExport{URL: e.GetUrl(), Title: e.GetTitle()})
	}
	return entries, nil
}

// FeedLinkAvailabilityGateway is the collector's poll-health state machine
// (catalog §2.G).
//
// Two methods where the driver had three. IncrementFeedLinkFailures and
// DisableFeedLink are not exposed separately because the caller used them as a
// read-modify-write — increment, inspect the count, disable — and the whole
// point of RecordFeedLinkFailure is that the inspection happens inside the
// provider's transaction (catalog §4-4). Keeping the split methods available
// here would leave the racy sequence one autocomplete away.
type FeedLinkAvailabilityGateway struct {
	client datahubv1connect.DataHubServiceClient
	// disableAfterFailures is how many consecutive failures the operator will
	// tolerate. It lives on this side because it is policy, not an invariant:
	// the provider applies whatever number it is handed, atomically.
	disableAfterFailures int
}

func NewFeedLinkAvailabilityGateway(client datahubv1connect.DataHubServiceClient, disableAfterFailures int) *FeedLinkAvailabilityGateway {
	if client == nil {
		panic("datahub_gateway: FeedLinkAvailabilityGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	if disableAfterFailures <= 0 {
		panic("datahub_gateway: FeedLinkAvailabilityGateway requires a positive failure threshold — " +
			"zero would mean broken feeds are polled forever, and that should be a deliberate config value, not a forgotten argument")
	}
	return &FeedLinkAvailabilityGateway{client: client, disableAfterFailures: disableAfterFailures}
}

// RecordFeedLinkFailure counts the failure and disables the link in the same
// provider transaction once the run reaches the configured threshold.
//
// The bool reports the transition to inactive, so the caller logs the
// auto-disable once rather than on every subsequent poll of a dead feed.
func (g *FeedLinkAvailabilityGateway) RecordFeedLinkFailure(ctx context.Context, feedURL, reason string) (*domain.FeedLinkAvailability, bool, error) {
	resp, err := g.client.RecordFeedLinkFailure(ctx, connect.NewRequest(&datahubv1.RecordFeedLinkFailureRequest{
		FeedUrl:              feedURL,
		Reason:               reason,
		DisableAfterFailures: safeconv.Int32(g.disableAfterFailures),
	}))
	if err != nil {
		return nil, false, fmt.Errorf("record feed link failure for %q: %w", feedURL, err)
	}

	availability, convErr := feedLinkAvailabilityFromProto(resp.Msg.GetAvailability())
	if convErr != nil {
		return nil, false, convErr
	}
	return availability, resp.Msg.GetDisabledNow(), nil
}

func (g *FeedLinkAvailabilityGateway) ResetFeedLinkFailures(ctx context.Context, feedURL string) error {
	_, err := g.client.ResetFeedLinkFailures(ctx, connect.NewRequest(&datahubv1.ResetFeedLinkFailuresRequest{
		FeedUrl: feedURL,
	}))
	if err != nil {
		return fmt.Errorf("reset feed link failures for %q: %w", feedURL, err)
	}
	return nil
}

func feedLinkFromProto(l *datahubv1.FeedLink) (*domain.FeedLink, error) {
	if l == nil {
		return nil, fmt.Errorf("feed link message is missing")
	}
	id, err := parseUUID(l.GetId())
	if err != nil {
		return nil, fmt.Errorf("feed link id: %w", err)
	}
	return &domain.FeedLink{ID: id, URL: l.GetUrl()}, nil
}

func feedLinkAvailabilityFromProto(a *datahubv1.FeedLinkAvailability) (*domain.FeedLinkAvailability, error) {
	if a == nil {
		return nil, nil
	}
	id, err := parseUUID(a.GetFeedLinkId())
	if err != nil {
		return nil, fmt.Errorf("feed link availability id: %w", err)
	}
	return &domain.FeedLinkAvailability{
		FeedLinkID:          id,
		IsActive:            a.GetIsActive(),
		ConsecutiveFailures: int(a.GetConsecutiveFailures()),
		LastFailureAt:       timePtrFromProto(a.GetLastFailureAt()),
		LastFailureReason:   a.LastFailureReason,
	}, nil
}
