package datahubapi

import (
	"errors"
	"fmt"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// maxFeedLimit bounds the cursor and limit walks over feeds. Same ceiling as
// the article walks; the value is the provider's, because a caller that asks
// for more than the provider will plan for should be told, not quietly served
// a shorter page it will mistake for the end of the list.
const maxFeedLimit = 500

// ---------------------------------------------------------------------------
// Conversions
// ---------------------------------------------------------------------------

// feedScopeFromProto refuses the unspecified value rather than defaulting to
// ALL. The four scopes return different sets, and a caller that forgot the
// field would otherwise be handed read feeds where it asked for unread ones —
// a wrong answer that looks like a right one.
func feedScopeFromProto(s datahubv1.FeedScope) (datahub_capability_port.FeedScope, error) {
	switch s {
	case datahubv1.FeedScope_FEED_SCOPE_ALL:
		return datahub_capability_port.FeedScopeAll, nil
	case datahubv1.FeedScope_FEED_SCOPE_UNREAD:
		return datahub_capability_port.FeedScopeUnread, nil
	case datahubv1.FeedScope_FEED_SCOPE_READ:
		return datahub_capability_port.FeedScopeRead, nil
	case datahubv1.FeedScope_FEED_SCOPE_FAVORITE:
		return datahub_capability_port.FeedScopeFavorite, nil
	default:
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("scope is required"))
	}
}

func parseUUIDs(raw []string, field string) ([]uuid.UUID, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("%s contains a value that is not a uuid: %w", field, err))
		}
		out = append(out, id)
	}
	return out, nil
}

func clampFeedLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > maxFeedLimit {
		return maxFeedLimit
	}
	return limit
}

func feedLinkAvailabilityToProto(a *domain.FeedLinkAvailability) *datahubv1.FeedLinkAvailability {
	if a == nil {
		return nil
	}
	out := &datahubv1.FeedLinkAvailability{
		FeedLinkId:          a.FeedLinkID.String(),
		IsActive:            a.IsActive,
		ConsecutiveFailures: safeconv.Int32(a.ConsecutiveFailures),
		LastFailureReason:   a.LastFailureReason,
	}
	if a.LastFailureAt != nil {
		out.LastFailureAt = timestamppb.New(*a.LastFailureAt)
	}
	return out
}

func feedSummaryToProto(s *domain.FeedSummary) *datahubv1.FeedSummary {
	if s == nil {
		return nil
	}
	return &datahubv1.FeedSummary{Summary: s.Summary}
}

func feedRowsToProto(rows []*domain.FeedRow) []*datahubv1.Feed {
	out := make([]*datahubv1.Feed, 0, len(rows))
	for _, r := range rows {
		if f := feedRowToProto(r); f != nil {
			out = append(out, f)
		}
	}
	return out
}

func feedRowToProto(r *domain.FeedRow) *datahubv1.Feed {
	if r == nil {
		return nil
	}
	return &datahubv1.Feed{
		Id:          r.ID,
		Title:       r.Title,
		Description: r.Description,
		WebsiteUrl:  r.WebsiteURL,
		// A zero pub_date stays an unset Timestamp. Many RSS items carry no
		// publication date and the driver scans the zero value; encoding it as
		// year 1 would make every such feed sort to the bottom of a client that
		// trusts the field.
		PubDate:    timestampOrNil(r.PubDate),
		CreatedAt:  timestampOrNil(r.CreatedAt),
		UpdatedAt:  timestampOrNil(r.UpdatedAt),
		ArticleId:  r.ArticleID,
		IsRead:     r.IsRead,
		FeedLinkId: r.FeedLinkID,
		OgImageUrl: r.OgImageURL,
	}
}
