package fetch_feed_gateway

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"alt/domain"
	"alt/orchestrator/driver/models"
	"alt/utils/logger"
)

// Every cursor walk selects f.id and every row carries it; this is the hop that
// decides whether it survives into the domain.
//
// It did not, and nothing noticed, because losing it costs nothing visible here
// — the feed list renders identically. The cost lands two layers away, in a
// ResolveOgImages call that matches no rows and returns 200 with an empty body.
// So the assertion has to be made where the value is dropped, not where the
// symptom appears.
func TestFetchFeedsGateway_CursorWalks_CarryFeedsDotID(t *testing.T) {
	logger.InitLogger()

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "reader@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	feedID := uuid.New()
	articleID := uuid.New().String()
	now := time.Now()

	newGateway := func() *FetchFeedsGateway {
		return &FetchFeedsGateway{store: &feedListStoreStub{feeds: []*models.Feed{{
			ID:          feedID.String(),
			Title:       "Title",
			Description: "Desc",
			WebsiteURL:  "https://example.com/post",
			PubDate:     now,
			CreatedAt:   now,
			UpdatedAt:   now,
			ArticleID:   &articleID,
		}}}}
	}

	// All four, because the two timeline scopes and the two membership scopes
	// are separate mappers that have drifted apart before, and a reader reaches
	// VisualFeedCard through any of them.
	walks := map[string]func(*FetchFeedsGateway) ([]*domain.FeedItem, error){
		"all": func(g *FetchFeedsGateway) ([]*domain.FeedItem, error) {
			return g.FetchFeedsListCursor(ctx, nil, 20, nil)
		},
		"unread": func(g *FetchFeedsGateway) ([]*domain.FeedItem, error) {
			return g.FetchUnreadFeedsListCursor(ctx, nil, 20, nil)
		},
		"read": func(g *FetchFeedsGateway) ([]*domain.FeedItem, error) {
			return g.FetchReadFeedsListCursor(ctx, nil, 20)
		},
		"favorite": func(g *FetchFeedsGateway) ([]*domain.FeedItem, error) {
			return g.FetchFavoriteFeedsListCursor(ctx, nil, 20)
		},
	}

	for name, walk := range walks {
		t.Run(name, func(t *testing.T) {
			items, err := walk(newGateway())
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.Equal(t, feedID, items[0].FeedID,
				"feeds.id was dropped on the %s cursor walk, so ResolveOgImages "+
					"has nothing to match and every card on this surface stays blank", name)
			require.Equal(t, articleID, items[0].ArticleID,
				"articles.id must still travel alongside it, not instead of it")
		})
	}
}

// A row whose id is not a UUID stops the page instead of arriving with an empty
// feed_id.
//
// feeds.id is a uuid PRIMARY KEY, so this is unreachable from a healthy alt-db
// and the point is what happens when it is not: an empty id would reach the
// browser, be dropped by the resolver without a request, and look exactly like
// a feed the publisher had no image for (CLAUDE.md rule 8 — an unwired path
// must not be indistinguishable from a working one).
func TestFetchFeedsGateway_CursorWalk_RefusesAnUnparseableFeedID(t *testing.T) {
	logger.InitLogger()

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "reader@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	now := time.Now()
	gateway := &FetchFeedsGateway{store: &feedListStoreStub{feeds: []*models.Feed{{
		ID:         "https://example.com/post", // a link, not a uuid
		Title:      "Title",
		WebsiteURL: "https://example.com/post",
		CreatedAt:  now,
		UpdatedAt:  now,
	}}}}

	items, err := gateway.FetchFeedsListCursor(ctx, nil, 20, nil)
	require.Error(t, err)
	require.Nil(t, items)
}
