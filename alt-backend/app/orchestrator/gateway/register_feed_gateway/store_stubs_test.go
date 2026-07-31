package register_feed_gateway

import (
	"context"
	"errors"

	"alt/domain"
	"alt/orchestrator/driver/models"
)

// Registration stopped owning a database with ADR-000954 Wave 3 batch 3
// (capability catalog §2.F W3-F1 / §2.H W3-H1). The tests these stubs replace
// worked by handing the gateways a nil *AltDBRepository and asserting
// "database connection not available" came back — an assertion that could only
// fail if someone deleted a nil check, and that passed identically for a
// gateway nobody had wired.
//
// feedLinkWriteStoreStub also records what it was handed, which is how the
// tracking-parameter strip is now verified: the sanitised URL is observable
// directly rather than inferred from a Begin error.

// errDataPlaneUnavailable stands in for whatever alt-data-hub returns when it
// cannot serve a capability. The tests match on identity rather than on the
// message so that rewording it does not silently stop asserting anything.
var errDataPlaneUnavailable = errors.New("data plane unavailable")

type feedLinkWriteStoreStub struct {
	gotLinks []string
	err      error
}

func (s *feedLinkWriteStoreStub) RegisterRSSFeedLink(_ context.Context, link string) error {
	s.gotLinks = append(s.gotLinks, link)
	return s.err
}

type feedWriteStoreStub struct {
	gotFeeds []models.Feed
	results  []domain.FeedRegistrationResult
	err      error
}

func (s *feedWriteStoreStub) RegisterMultipleFeedsWithState(_ context.Context, feeds []models.Feed) ([]domain.FeedRegistrationResult, error) {
	s.gotFeeds = feeds
	return s.results, s.err
}
