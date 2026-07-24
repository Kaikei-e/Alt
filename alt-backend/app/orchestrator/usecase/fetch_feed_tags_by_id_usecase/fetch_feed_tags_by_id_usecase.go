// Package fetch_feed_tags_by_id_usecase closes the Handler->Driver layer
// violation in RestHandleFetchFeedTagsByID (finding [10]): the handler is
// already given a feed ID (path parameter), so it does not need the
// URL->feedID resolution step fetch_feed_tags_usecase performs — but it must
// still go through Usecase->Port->Gateway rather than calling
// container.AltDBRepository directly.
package fetch_feed_tags_by_id_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/fetch_feed_tags_port"
	"alt/utils/logger"
	"context"
	"errors"
	"strings"
	"time"
)

type FetchFeedTagsByIDUsecase struct {
	fetchFeedTagsGateway fetch_feed_tags_port.FetchFeedTagsPort
}

func NewFetchFeedTagsByIDUsecase(fetchFeedTagsGateway fetch_feed_tags_port.FetchFeedTagsPort) *FetchFeedTagsByIDUsecase {
	return &FetchFeedTagsByIDUsecase{fetchFeedTagsGateway: fetchFeedTagsGateway}
}

func (u *FetchFeedTagsByIDUsecase) Execute(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error) {
	if strings.TrimSpace(feedID) == "" {
		logger.Logger.ErrorContext(ctx, "invalid feed_id: must not be empty")
		return nil, errors.New("feed_id must not be empty")
	}

	tags, err := u.fetchFeedTagsGateway.FetchFeedTags(ctx, feedID, cursor, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch feed tags by id", "error", err, "feedID", feedID)
		return nil, err
	}

	return tags, nil
}
