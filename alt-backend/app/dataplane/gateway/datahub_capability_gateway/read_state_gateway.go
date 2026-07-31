package datahub_capability_gateway

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"alt/domain"
	"alt/shared/driver/alt_db"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// §2.I Read state / subscriptions / favorites
// ---------------------------------------------------------------------------

// readStateDriver is the slice of alt_db the §2.I capabilities use.
//
// The two read-status writes appear as two methods on two embedded
// repositories, which is how the duplication in catalog §4-5 was visible in
// the first place. They now issue the same statement and answer
// domain.ErrFeedNotFound the same way; what is left is the lookup key.
type readStateDriver interface {
	UpdateFeedStatus(ctx context.Context, feedURL url.URL, userID uuid.UUID) error
	MarkArticleAsRead(ctx context.Context, articleURL url.URL, userID uuid.UUID) error
	GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error)
	GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	FetchSubscriptions(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error)
	InsertSubscription(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error
	DeleteSubscription(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error
	RegisterFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) error
	RemoveFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) error
}

// ReadStateGateway implements datahub_capability_port.ReadStatePort.
type ReadStateGateway struct {
	db readStateDriver
}

func NewReadStateGateway(db *alt_db.AltDBRepository) *ReadStateGateway {
	return &ReadStateGateway{db: db}
}

// MarkFeedRead parses the URL the request carried as a string.
//
// The driver still takes a url.URL, which was free when the caller had already
// parsed one; across the boundary the URL arrives as text and this is where it
// becomes a URL again. A parse failure is the caller's error, not the
// database's, and the handler maps it to InvalidArgument.
func (g *ReadStateGateway) MarkFeedRead(ctx context.Context, feedURL string, userID uuid.UUID) error {
	parsed, err := url.Parse(feedURL)
	if err != nil {
		return fmt.Errorf("parse feed url %q: %w", feedURL, err)
	}
	// The domain error travels unwrapped: the handler distinguishes NotFound
	// from Internal with errors.Is, and %w through a fmt.Errorf here would
	// still satisfy it — but the message would claim a failure where the
	// answer is simply "no such feed".
	return g.db.UpdateFeedStatus(ctx, *parsed, userID)
}

func (g *ReadStateGateway) MarkArticleRead(ctx context.Context, articleURL string, userID uuid.UUID) error {
	parsed, err := url.Parse(articleURL)
	if err != nil {
		return fmt.Errorf("parse article url %q: %w", articleURL, err)
	}
	return g.db.MarkArticleAsRead(ctx, *parsed, userID)
}

// ReadFeedIDs turns the driver's set-shaped map into the list the message
// carries.
//
// The map was a lookup table for a caller in the same address space. Over the
// wire it would be a map of feed id to true, where every value is the same and
// the absence of a key is the whole meaning — so the list is both smaller and
// the honest shape.
func (g *ReadStateGateway) ReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) ([]uuid.UUID, error) {
	read, err := g.db.GetReadFeedIDs(ctx, userID, feedIDs)
	if err != nil {
		return nil, fmt.Errorf("get read feed ids for user %s: %w", userID, err)
	}
	return readIDList(read), nil
}

func (g *ReadStateGateway) AllReadFeedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	read, err := g.db.GetAllReadFeedIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get all read feed ids for user %s: %w", userID, err)
	}
	return readIDList(read), nil
}

// readIDList keeps only the ids marked read. The driver never stores false,
// but reading the value rather than assuming it means a future "unread"
// marker cannot leak through as read.
func readIDList(read map[uuid.UUID]bool) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(read))
	for id, isRead := range read {
		if isRead {
			ids = append(ids, id)
		}
	}
	return ids
}

func (g *ReadStateGateway) SubscribedFeedLinkIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := g.db.GetUserSubscriptions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get subscribed feed link ids for user %s: %w", userID, err)
	}
	return ids, nil
}

func (g *ReadStateGateway) ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error) {
	sources, err := g.db.FetchSubscriptions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions for user %s: %w", userID, err)
	}
	return sources, nil
}

func (g *ReadStateGateway) Subscribe(ctx context.Context, userID, feedLinkID uuid.UUID) error {
	if err := g.db.InsertSubscription(ctx, userID, feedLinkID); err != nil {
		return fmt.Errorf("subscribe user %s to feed link %s: %w", userID, feedLinkID, err)
	}
	return nil
}

func (g *ReadStateGateway) Unsubscribe(ctx context.Context, userID, feedLinkID uuid.UUID) error {
	if err := g.db.DeleteSubscription(ctx, userID, feedLinkID); err != nil {
		return fmt.Errorf("unsubscribe user %s from feed link %s: %w", userID, feedLinkID, err)
	}
	return nil
}

// AddFavorite and RemoveFavorite pass the driver's pgx.ErrNoRows through
// unwrapped, for the same reason MarkFeedRead passes domain.ErrFeedNotFound
// through: the handler tells NotFound from Internal by inspecting it, and a
// wrapping message would describe a fault where there is only an absence.
func (g *ReadStateGateway) AddFavorite(ctx context.Context, feedURL string, userID uuid.UUID) error {
	return g.db.RegisterFavoriteFeed(ctx, feedURL, userID)
}

func (g *ReadStateGateway) RemoveFavorite(ctx context.Context, feedURL string, userID uuid.UUID) error {
	return g.db.RemoveFavoriteFeed(ctx, feedURL, userID)
}

// ---------------------------------------------------------------------------
// §2.J Tag reads
// ---------------------------------------------------------------------------

type tagReadDriver interface {
	FetchArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error)
	FetchFeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error)
	FetchTagCooccurrences(ctx context.Context, tagNames []string) ([]*domain.TagCooccurrence, error)
	SearchTagsByPrefix(ctx context.Context, prefix string, limit int) ([]domain.GlobalTagHit, error)
	FetchTagArticleCounts(ctx context.Context, userID uuid.UUID, since time.Time) ([]domain.TagArticleCount, error)
}

// TagReadGateway implements datahub_capability_port.TagReadPort.
type TagReadGateway struct {
	db tagReadDriver
}

func NewTagReadGateway(db *alt_db.AltDBRepository) *TagReadGateway {
	return &TagReadGateway{db: db}
}

func (g *TagReadGateway) ArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	tags, err := g.db.FetchArticleTags(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("get article tags %s: %w", articleID, err)
	}
	return tags, nil
}

func (g *TagReadGateway) FeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error) {
	tags, err := g.db.FetchFeedTags(ctx, feedID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("get feed tags %s: %w", feedID, err)
	}
	return tags, nil
}

func (g *TagReadGateway) Cooccurrences(ctx context.Context, tagNames []string) ([]*domain.TagCooccurrence, error) {
	items, err := g.db.FetchTagCooccurrences(ctx, tagNames)
	if err != nil {
		return nil, fmt.Errorf("get tag cooccurrences: %w", err)
	}
	return items, nil
}

func (g *TagReadGateway) SearchByPrefix(ctx context.Context, prefix string, limit int) ([]domain.GlobalTagHit, error) {
	hits, err := g.db.SearchTagsByPrefix(ctx, prefix, limit)
	if err != nil {
		return nil, fmt.Errorf("search tags by prefix %q: %w", prefix, err)
	}
	return hits, nil
}

func (g *TagReadGateway) ArticleCounts(ctx context.Context, userID uuid.UUID, since time.Time) ([]domain.TagArticleCount, error) {
	counts, err := g.db.FetchTagArticleCounts(ctx, userID, since)
	if err != nil {
		return nil, fmt.Errorf("get tag article counts for user %s: %w", userID, err)
	}
	return counts, nil
}
