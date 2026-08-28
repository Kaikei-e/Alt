package feeds

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/orchestrator/usecase/og_image_resolve_usecase"
)

// This file pins one thing, and it is the thing PR #232 shipped broken: the
// identifier a feed list hands the browser must be an identifier
// FetchFeedOgImageTargets will match.
//
// Those are two different tables. The resolver's WHERE clause is
// `WHERE f.id = ANY($1::uuid[])` over *feeds* (shared/driver/alt_db/
// feed_og_image_driver.go), while FeedItem.Id has always been articles.id —
// or, when no article row exists, a UUID minted per response purely so a
// Svelte `{#each}` key is unique. Neither is a feeds.id, so every
// ResolveOgImages call matched zero rows and the RPC succeeded with an empty
// body. Nothing failed; the pictures simply never arrived.
//
// A test that only asserted on convertFeedsToProto's output could not have
// caught that — the emitted id looked perfectly well-formed. The catch has to
// be the join: take the identifier the client is handed and offer it to the
// store the resolver actually queries.

// ogImageResolveFieldName is the FeedItem field alt-frontend-sv sends to
// ResolveOgImages — see `ogImageResolver().resolve(...)` in
// VisualFeedCard.svelte and MobileGalleryTile.svelte.
//
// It is looked up by name through protoreflect rather than through the
// generated getter on purpose. A missing field then fails this test as an
// assertion naming the absent field, which is a description of the bug, instead
// of failing the package to compile, which is a description of nothing.
const ogImageResolveFieldName = "feed_id"

func ogImageResolveKey(t *testing.T, item *feedsv2.FeedItem) string {
	t.Helper()

	msg := item.ProtoReflect()
	fd := msg.Descriptor().Fields().ByName(ogImageResolveFieldName)
	require.NotNilf(t, fd,
		"alt.feeds.v2.FeedItem has no %q field, so the browser has nothing but `id` to send to "+
			"ResolveOgImages — and `id` is articles.id, which `WHERE f.id = ANY($1::uuid[])` never matches",
		ogImageResolveFieldName)
	return msg.Get(fd).String()
}

// feedsTableStore is FetchFeedOgImageTargets' WHERE clause in Go.
//
// Two properties of the real query are reproduced because both are load-bearing
// here. A row comes back only under its feeds.id, which is what makes a
// wrong-table identifier miss; and an identifier that is not a UUID is an
// error, not a miss, because `$1::uuid[]` is a cast Postgres performs before it
// compares anything. The second is why the frontend's SSR path — which puts a
// *link URL* in `id` when a feed has no article (sanitize.ts) — would take this
// RPC to 5xx rather than to silence.
type feedsTableStore struct {
	rows  map[string]domain.FeedOgImageTarget
	asked []string
}

func (s *feedsTableStore) FetchFeedOgImageTargets(_ context.Context, feedIDs []string) ([]domain.FeedOgImageTarget, error) {
	s.asked = append(s.asked, feedIDs...)

	out := make([]domain.FeedOgImageTarget, 0, len(feedIDs))
	for _, id := range feedIDs {
		if _, err := uuid.Parse(id); err != nil {
			return nil, fmt.Errorf("invalid input syntax for type uuid: %q", id)
		}
		if row, ok := s.rows[id]; ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func (s *feedsTableStore) SaveFeedOgImage(
	_ context.Context, _, _ string, _ domain.OgImageRefusal, _ time.Duration,
) error {
	return nil
}

func newFeedsTableStore(targets ...domain.FeedOgImageTarget) *feedsTableStore {
	rows := make(map[string]domain.FeedOgImageTarget, len(targets))
	for _, t := range targets {
		rows[t.FeedID] = t
	}
	return &feedsTableStore{rows: rows}
}

// resolveHandlerOver wires the real handler over a store that answers only to
// feeds.id, so the assertion below is about the identifier and nothing else.
func resolveHandlerOver(store *feedsTableStore, pages map[string]string) *Handler {
	return NewHandler(
		FeedHandlerDeps{ResolveOgImages: og_image_resolve_usecase.NewUsecase(
			store,
			&resolveFakeFetcher{byURL: pages},
			resolveFakeMinter{},
		)},
		&config.Config{},
		slog.Default(),
	)
}

// The whole round trip, for the row that matters most: a feed with no article.
//
// Feeds needing on-demand resolution are overwhelmingly the ones the RSS gave
// no image and no article row for, which is exactly where FeedItem.Id degrades
// to a per-response UUID. If the identifier the client is handed is going to
// miss anywhere, it misses here.
func TestFeedListHandsOutAnIDResolveOgImagesAccepts_NoArticle(t *testing.T) {
	feedID := uuid.MustParse("11111111-1111-4111-8111-111111111111")

	items := convertFeedsToProto([]*domain.FeedItem{{
		FeedID:          feedID,
		Title:           "A post whose RSS carried no image",
		Link:            "https://example.com/post",
		PublishedParsed: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}})
	require.Len(t, items, 1)

	store := newFeedsTableStore(domain.FeedOgImageTarget{
		FeedID:  feedID.String(),
		PageURL: "https://example.com/post",
	})
	h := resolveHandlerOver(store, map[string]string{
		"https://example.com/post": "https://cdn.example.com/post.png",
	})

	key := ogImageResolveKey(t, items[0])
	assert.Equal(t, feedID.String(), key,
		"the identifier handed to the browser must be the feeds.id the resolver queries")

	resp, err := h.ResolveOgImages(authedContext(), connect.NewRequest(&feedsv2.ResolveOgImagesRequest{
		FeedIds: []string{key},
	}))
	require.NoError(t, err)

	require.Len(t, resp.Msg.GetImages(), 1,
		"ResolveOgImages matched nothing for the id the feed list handed out — "+
			"asked for %v, feeds table holds %v", store.asked, keysOf(store.rows))
	assert.Equal(t, feedID.String(), resp.Msg.GetImages()[0].GetFeedId())
	assert.Equal(t, "/proxy?u=https://cdn.example.com/post.png",
		resp.Msg.GetImages()[0].GetOgImageProxyUrl())
}

// The same round trip for a feed that does have an article, plus the reason
// this is an additive field rather than a redefinition of `id`.
//
// `id` stays articles.id: it is the `{#each}` key on FeedGrid, the search
// results list and the dashboard widget, and the identity `appendUniqueFeeds`
// dedupes on. SearchFeeds builds its FeedItems out of Meilisearch hits and has
// no feeds.id to offer at all (search_feed_meilisearch_usecase.go), so moving
// `id` to feeds.id would key that list entirely on empty strings — one
// each_key_duplicate crash, not a missing picture.
func TestFeedListHandsOutAnIDResolveOgImagesAccepts_WithArticle(t *testing.T) {
	feedID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	articleID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	items := convertFeedsToProto([]*domain.FeedItem{{
		FeedID:          feedID,
		ArticleID:       articleID.String(),
		Title:           "A post with an article row",
		Link:            "https://example.com/read",
		PublishedParsed: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}})
	require.Len(t, items, 1)

	assert.Equal(t, articleID.String(), items[0].GetId(),
		"`id` is the Svelte keying identity and mark-as-read's article handle; it must not move")
	assert.Equal(t, articleID.String(), items[0].GetArticleId())

	store := newFeedsTableStore(domain.FeedOgImageTarget{
		FeedID:  feedID.String(),
		PageURL: "https://example.com/read",
	})
	h := resolveHandlerOver(store, map[string]string{
		"https://example.com/read": "https://cdn.example.com/read.png",
	})

	key := ogImageResolveKey(t, items[0])
	assert.Equal(t, feedID.String(), key)
	assert.NotEqual(t, items[0].GetId(), key,
		"articles.id and feeds.id are independent PKs on separate tables; if these are ever "+
			"equal the test is asserting nothing")

	resp, err := h.ResolveOgImages(authedContext(), connect.NewRequest(&feedsv2.ResolveOgImagesRequest{
		FeedIds: []string{key},
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetImages(), 1,
		"ResolveOgImages matched nothing — asked for %v, feeds table holds %v",
		store.asked, keysOf(store.rows))
	assert.Equal(t, "/proxy?u=https://cdn.example.com/read.png",
		resp.Msg.GetImages()[0].GetOgImageProxyUrl())
}

// A feed the list could not name a feeds.id for is handed an empty key rather
// than a plausible-looking one.
//
// The empty string is the one value the browser's resolver short-circuits on
// (ogImageResolver.ts returns `absent` without sending), so it costs no RPC.
// uuid.Nil would cost one: it is a well-formed UUID, it clears the `::uuid[]`
// cast, and it would arrive at the store as an ordinary miss — which is
// indistinguishable from the bug this file exists to prevent.
func TestFeedListEmitsNoResolveKeyWhenFeedIDIsUnknown(t *testing.T) {
	items := convertFeedsToProto([]*domain.FeedItem{{
		Title:           "A Meilisearch hit, which has no feeds.id",
		Link:            "https://example.com/hit",
		ArticleID:       "44444444-4444-4444-8444-444444444444",
		PublishedParsed: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}})
	require.Len(t, items, 1)

	assert.Empty(t, ogImageResolveKey(t, items[0]),
		"an absent feeds.id must travel as empty, never as uuid.Nil")
}

func keysOf(m map[string]domain.FeedOgImageTarget) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
