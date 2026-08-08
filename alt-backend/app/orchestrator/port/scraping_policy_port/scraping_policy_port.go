package scraping_policy_port

import (
	"context"
	"errors"
	"fmt"
	"time"
)

//go:generate go run go.uber.org/mock/mockgen -source=scraping_policy_port.go -destination=../../mocks/mock_scraping_policy_port.go

// ErrCrawlDelayNotElapsed reports that the publisher's robots.txt Crawl-delay
// has not elapsed since the last fetch of this domain.
//
// This is a *transient* refusal and must stay distinguishable from a permanent
// one. A bare (false, nil) reads identically to a robots.txt disallow, and the
// caller responds to a disallow by recording the domain in declined_domains —
// a table with no expiry and no delete path. Swiping quickly through one
// publisher's feed therefore banned that publisher for that user forever.
var ErrCrawlDelayNotElapsed = errors.New("crawl delay has not elapsed")

// CrawlDelayError carries how long the caller must wait, so the refusal can be
// turned into a Retry-After the client can actually act on.
type CrawlDelayError struct {
	Domain     string
	RetryAfter time.Duration
}

func (e *CrawlDelayError) Error() string {
	return fmt.Sprintf("crawl delay for %s: %s remaining", e.Domain, e.RetryAfter)
}

func (e *CrawlDelayError) Unwrap() error { return ErrCrawlDelayNotElapsed }

// ScrapingPolicyPort defines the interface for scraping policy checks
type ScrapingPolicyPort interface {
	// CanFetchArticle reports whether an article URL may be fetched, based on
	// domain policy and robots.txt.
	//
	// A false result paired with ErrCrawlDelayNotElapsed is transient: retry
	// later, and do not record the domain as declined. A false result with a nil
	// error is a permanent policy refusal.
	CanFetchArticle(ctx context.Context, articleURL string) (bool, error)
}
