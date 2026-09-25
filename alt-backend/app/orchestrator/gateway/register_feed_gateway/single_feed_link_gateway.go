package register_feed_gateway

import (
	"alt/utils"
	"alt/utils/logger"
	"context"
	stderrors "errors"
)

// RegisterFeedGateway handles DB-only feed link registration.
// FeedLinkWriteStore is the one capability feed registration needs from
// alt-data-hub (capability catalog §2.F W3-F1). Registration is idempotent
// there: an already-subscribed URL is reported, not raised.
type FeedLinkWriteStore interface {
	RegisterRSSFeedLink(ctx context.Context, link string) error
}

type RegisterFeedGateway struct {
	feedLinkStore FeedLinkWriteStore
}

// NewRegisterFeedLinkGateway builds the DB-only half of feed registration.
//
// It takes a store rather than a pool: since ADR-000954 Wave 3 batch 3 the
// insert happens in alt-data-hub, and the only thing that stays here is the
// tracking-parameter strip, which is a pure function over the URL (D4).
func NewRegisterFeedLinkGateway(store FeedLinkWriteStore) *RegisterFeedGateway {
	if store == nil {
		panic("register_feed_gateway: FeedLinkWriteStore is required (see .claude/rules/di-wiring.md)")
	}
	return &RegisterFeedGateway{feedLinkStore: store}
}

// RegisterFeedLink inserts a feed link URL into the database.
// This is a DB-only operation — no external HTTP fetching.
// Tracking parameters (utm_source, fbclid, etc.) are stripped before registration.
func (g *RegisterFeedGateway) RegisterFeedLink(ctx context.Context, link string) error {
	sanitized, sanitizeErr := utils.StripTrackingParams(link)
	if sanitizeErr != nil {
		logger.SafeWarnContext(ctx, "Failed to strip tracking params, using original", "error", sanitizeErr)
		sanitized = link
	}

	if err := g.feedLinkStore.RegisterRSSFeedLink(ctx, sanitized); err != nil {
		logger.SafeErrorContext(ctx, "Error registering RSS feed link", "error", err)
		return stderrors.New("failed to register RSS feed link")
	}
	logger.SafeInfoContext(ctx, "RSS feed link registered", "link", sanitized)
	return nil
}
