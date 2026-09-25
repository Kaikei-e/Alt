package datahubapi

import (
	"time"

	"alt/dataplane/port/internal_tag_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// maxTagLimit and defaultTagLimit bound the tag walks. The ceiling is the
// provider's rather than each caller's, because it is a statement about what
// this database will plan for; the default is what the in-process callers
// passed when they omitted one.
const (
	maxTagLimit     = 500
	defaultTagLimit = 100
)

// ---------------------------------------------------------------------------
// Conversion
// ---------------------------------------------------------------------------

// cursorFromProto turns an unset timestamp into a nil cursor rather than into
// the zero time. The port reads nil as "first page" and builds a query with no
// upper bound; the zero time would build `created_at < '0001-01-01'`, which
// selects nothing and looks like an empty tag.
func cursorFromProto(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func tagItemsFromProto(protoTags []*datahubv1.TagItem) []internal_tag_port.TagItem {
	tags := make([]internal_tag_port.TagItem, len(protoTags))
	for i, t := range protoTags {
		tags[i] = internal_tag_port.TagItem{Name: t.Name, Confidence: t.Confidence}
	}
	return tags
}

func tagTrailArticlesToProto(articles []*domain.TagTrailArticle) []*datahubv1.TagTrailArticle {
	out := make([]*datahubv1.TagTrailArticle, 0, len(articles))
	for _, a := range articles {
		if a == nil {
			continue
		}
		out = append(out, &datahubv1.TagTrailArticle{
			Id:          a.ID,
			Title:       a.Title,
			Url:         a.Link,
			PublishedAt: timestamppb.New(a.PublishedAt),
			FeedId:      a.FeedID,
			FeedTitle:   a.FeedTitle,
		})
	}
	return out
}

// clampTagLimit applies the provider's page ceiling, matching clampFeedLimit
// in batch 3 rather than introducing a second convention for the same
// question. An unset or non-positive limit gets the default the in-process
// callers passed.
func clampTagLimit(limit int32) int {
	if limit <= 0 {
		return defaultTagLimit
	}
	if int(limit) > maxTagLimit {
		return maxTagLimit
	}
	return int(limit)
}

func feedTagsToProto(tags []*domain.FeedTag) []*datahubv1.FeedTag {
	out := make([]*datahubv1.FeedTag, 0, len(tags))
	for _, t := range tags {
		if t == nil {
			continue
		}
		pb := &datahubv1.FeedTag{
			Id:         t.ID,
			FeedId:     t.FeedID,
			TagName:    t.TagName,
			Confidence: t.Confidence,
			TagType:    t.TagType,
		}
		// Zero times stay absent rather than becoming 1970. The article-tag
		// read does not select feed_id or updated_at, and a consumer that saw
		// an epoch timestamp would sort on it.
		if !t.CreatedAt.IsZero() {
			pb.CreatedAt = timestamppb.New(t.CreatedAt)
		}
		if !t.UpdatedAt.IsZero() {
			pb.UpdatedAt = timestamppb.New(t.UpdatedAt)
		}
		out = append(out, pb)
	}
	return out
}
