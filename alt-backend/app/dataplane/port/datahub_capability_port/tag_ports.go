package datahub_capability_port

import (
	"context"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

// TagReadPort is the read half of the tag tables (catalog §2.J).
//
// Reads only. UpsertArticleTags — the write that on-the-fly generation
// performs after mq-hub answers — is already a procedure of its own from
// the tag write capability, and the mq-hub call between the two stays with the caller
// (ADR-000954 D4).
type TagReadPort interface {
	// ArticleTags returns an article's tags. An untagged article is an empty
	// slice, not an error: the caller reads emptiness as "generate some".
	ArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error)
	// FeedTags pages one feed's tags newest first. A nil cursor is the first
	// page.
	FeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error)
	// Cooccurrences returns tag pairs sharing articles — the edge set the tag
	// cloud's layout consumes. The layout itself is a pure function and stays
	// with the caller.
	Cooccurrences(ctx context.Context, tagNames []string) ([]*domain.TagCooccurrence, error)
	// SearchByPrefix is the global search box's tag section.
	SearchByPrefix(ctx context.Context, prefix string, limit int) ([]domain.GlobalTagHit, error)
	// ArticleCounts counts one user's tagged articles since a timestamp.
	// Counts, not trending tags: which of them is a surge is arithmetic the
	// caller owns.
	ArticleCounts(ctx context.Context, userID uuid.UUID, since time.Time) ([]domain.TagArticleCount, error)
}

// TagTrailPort pages the Tag Trail (catalog §2.J, ADR-000954 Tag Trail capability).
//
// This is the last read alt-backend performed against its own pool that no
// existing procedure could express. FetchArticlesByTag — the earlier TagQuery procedure
// with the similar name — takes a tag *name*, has no cursor and returns four
// fields; the Tag Trail pages by feed_tags row id and reads six. Routing the
// caller through the older shape would have compiled and would have dropped
// its paging silently, which is why this port exists rather than a DI edit.
//
// A nil cursor means the first page. The two methods stay separate because the
// queries differ — the by-name one joins through feed_tags and deduplicates —
// and because merging them would need a third state meaning "neither
// identifier given".
type TagTrailPort interface {
	// ArticlesByTagID pages one feed tag's articles, newest first.
	ArticlesByTagID(ctx context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error)
	// ArticlesByTagName pages every feed's articles carrying a tag of this
	// name, deduplicated.
	ArticlesByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error)
}
