package fetch_feed_gateway

import (
	"context"
	"time"

	"alt/domain"
	"alt/orchestrator/driver/models"

	"github.com/google/uuid"
)

// The gateways in this package stopped owning a database with ADR-000954
// Wave 3 batch 3: the reads are alt-data-hub capabilities now (capability
// catalog §2.F / §2.H) and only the RSS fetch stays here.
//
// The tests these stubs replace all worked by handing the gateway a nil
// *AltDBRepository and asserting that "database connection not available" came
// back. That assertion could only fail if someone deleted a nil check, and it
// passed just as happily for a gateway nobody had wired — the exact confusion
// CLAUDE.md rule 8 exists to prevent. A store that returns the error under
// test says the same thing about error propagation without pretending a nil
// dependency is a runtime state.

type feedListStoreStub struct {
	feeds []*models.Feed
	err   error
}

func (s *feedListStoreStub) FetchFeedsList(context.Context) ([]*models.Feed, error) {
	return s.feeds, s.err
}

func (s *feedListStoreStub) FetchFeedsListLimit(context.Context, int) ([]*models.Feed, error) {
	return s.feeds, s.err
}

func (s *feedListStoreStub) FetchUnreadFeedsListPage(context.Context, int) ([]*models.Feed, error) {
	return s.feeds, s.err
}

func (s *feedListStoreStub) FetchAllFeedsListCursor(context.Context, *time.Time, int, []uuid.UUID) ([]*models.Feed, error) {
	return s.feeds, s.err
}

func (s *feedListStoreStub) FetchUnreadFeedsListCursor(context.Context, *time.Time, int, []uuid.UUID) ([]*models.Feed, error) {
	return s.feeds, s.err
}

func (s *feedListStoreStub) FetchReadFeedsListCursor(context.Context, *time.Time, int) ([]*models.Feed, error) {
	return s.feeds, s.err
}

func (s *feedListStoreStub) FetchFavoriteFeedsListCursor(context.Context, *time.Time, int) ([]*models.Feed, error) {
	return s.feeds, s.err
}

type pollableFeedLinkStoreStub struct {
	links []domain.FeedLink
	err   error
}

func (s *pollableFeedLinkStoreStub) FetchRSSFeedURLs(context.Context) ([]domain.FeedLink, error) {
	return s.links, s.err
}
