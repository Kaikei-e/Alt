package feed_url_to_id_gateway

import (
	"context"
	"errors"
	"fmt"

	"alt/utils/feed_url_normalize"
	"alt/utils/logger"

	"connectrpc.com/connect"
)

// feedIDResolver is the alt-data-hub lookup this gateway wraps
// (capability catalog W2-10, GetFeedID).
type feedIDResolver interface {
	GetFeedIDByURL(ctx context.Context, feedURL string) (string, error)
}

// FeedURLToIDGateway resolves a feed URL to its id, with one retry on the
// canonical form of the URL.
//
// The retry is why this type still exists after ADR-000954 Wave 3 moved the
// lookup itself behind DataHubService: it is flow orchestration — two attempts
// with a pure string transformation between them — and D4 keeps that on the
// calling side. The provider answers one question about one URL.
type FeedURLToIDGateway struct {
	feeds feedIDResolver
}

func NewFeedURLToIDGateway(feeds feedIDResolver) *FeedURLToIDGateway {
	if feeds == nil {
		panic("feed_url_to_id_gateway: a feed id resolver is required — " +
			"a nil one would report every feed as unregistered, which is the " +
			"exact symptom of the 2026-05-26 incident this retry was added for " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &FeedURLToIDGateway{feeds: feeds}
}

func (g *FeedURLToIDGateway) GetFeedIDByURL(ctx context.Context, feedURL string) (string, error) {
	logger.Logger.InfoContext(ctx, "getting feed ID by URL", "feedURL", feedURL)

	// Literal match first — preserves behaviour for any URL the registrar
	// stored in non-canonical form.
	feedID, err := g.feeds.GetFeedIDByURL(ctx, feedURL)
	if err == nil {
		logger.Logger.InfoContext(ctx, "successfully retrieved feed ID", "feedID", feedID)
		return feedID, nil
	}
	if !isFeedNotRegistered(err) {
		logger.Logger.ErrorContext(ctx, "failed to get feed ID by URL", "error", err, "feedURL", feedURL)
		return "", fmt.Errorf("GetFeedID: %w", err)
	}

	// Pillar 4 defense in depth: retry once with the canonical normalization.
	// The 2026-05-26 incident is feed-not-registered, but this branch makes
	// future trailing-slash / case / port drift a no-op instead of a 262-line
	// burst the operator has to parse out of the log.
	normalized := feed_url_normalize.Normalize(feedURL)
	if normalized == "" || normalized == feedURL {
		logger.Logger.InfoContext(ctx, "feed not registered", "feedURL", feedURL)
		return "", err
	}
	feedID, err2 := g.feeds.GetFeedIDByURL(ctx, normalized)
	if err2 == nil {
		logger.Logger.InfoContext(ctx, "successfully retrieved feed ID via normalized fallback",
			"feedURL", feedURL, "normalized", normalized, "feedID", feedID)
		return feedID, nil
	}
	if !isFeedNotRegistered(err2) {
		logger.Logger.ErrorContext(ctx, "failed to get feed ID by normalized URL",
			"error", err2, "feedURL", feedURL, "normalized", normalized)
		return "", fmt.Errorf("GetFeedID (normalized): %w", err2)
	}
	logger.Logger.InfoContext(ctx, "feed not registered", "feedURL", feedURL, "normalized", normalized)
	return "", err
}

// isFeedNotRegistered distinguishes "no feed at this URL" — the only case worth
// a second attempt — from a fault.
//
// It reads the Connect code where the driver version read
// alt_db.ErrFeedNotFoundByURL. Being wrong in either direction is visible:
// treating a fault as an absence turns one failed round trip into two, and
// treating an absence as a fault skips the normalization retry that exists
// because a URL differing only by a trailing slash used to look unregistered.
func isFeedNotRegistered(err error) bool {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr.Code() == connect.CodeNotFound
	}
	return false
}
