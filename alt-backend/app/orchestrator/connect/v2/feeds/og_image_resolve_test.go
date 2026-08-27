package feeds

import (
	"context"
	"errors"
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

// This file is the producer half of the `ResolveOgImages` wire contract: it
// pins that alt-backend emits the four outcomes documented on
// ResolveOgImagesResponse in proto/alt/feeds/v2/feeds.proto, in the encoding
// that comment specifies.
//
// It is a handler test rather than a Pact provider verification because
// alt-frontend-sv — the only consumer of this RPC — is not a Pact pacticipant
// in this repository: its `src/test/contracts/*.test.ts` are vitest
// proto-shape round-trips, and no `alt-frontend-sv-alt-backend.json` exists for
// alt-backend's provider suite to verify against. Until a consumer-side pact
// exists there, these assertions are the only thing standing between the four
// outcomes and a client that collapses them into one.

type resolveFakeStore struct {
	targets []domain.FeedOgImageTarget
}

func (s *resolveFakeStore) FetchFeedOgImageTargets(_ context.Context, _ []string) ([]domain.FeedOgImageTarget, error) {
	return s.targets, nil
}

func (s *resolveFakeStore) SaveFeedOgImage(
	_ context.Context, _, _ string, _ domain.OgImageRefusal, _ time.Duration,
) error {
	return nil
}

type resolveFakeFetcher struct {
	byURL    map[string]string
	refusals map[string]domain.OgImageRefusal
	errs     map[string]error
}

func (f *resolveFakeFetcher) FetchOgImage(_ context.Context, pageURL string) (string, domain.OgImageRefusal, error) {
	if err, ok := f.errs[pageURL]; ok {
		return "", "", err
	}
	if r, ok := f.refusals[pageURL]; ok {
		return "", r, nil
	}
	return f.byURL[pageURL], "", nil
}

type resolveFakeMinter struct{}

func (resolveFakeMinter) GenerateProxyURL(imageURL string) string { return "/proxy?u=" + imageURL }
func (resolveFakeMinter) WarmCache(_ context.Context, _ string)   {}

func resolveTestHandler(uc *og_image_resolve_usecase.Usecase) *Handler {
	return NewHandler(
		FeedHandlerDeps{ResolveOgImages: uc},
		&config.Config{},
		slog.Default(),
	)
}

func authedContext() context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "reader@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

// All four outcomes in one response, because the contract is about how they are
// told apart and a test that saw them one at a time could not fail on a
// collapse.
func TestResolveOgImages_SeparatesTheFourOutcomes(t *testing.T) {
	const (
		resolvedFeed  = "feed-resolved"
		barredFeed    = "feed-barred"
		settledFeed   = "feed-settled"
		ourFaultFeed  = "feed-our-fault"
		neverSeenFeed = "feed-never-seen"
	)

	store := &resolveFakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: resolvedFeed, PageURL: "https://example.com/ok"},
		// Already refused once; four of the five seconds are left.
		{FeedID: barredFeed, PageURL: "https://example.com/barred", Suppressed: true, Attempts: 1, RetryAfterSeconds: 4},
		// robots.txt: settled for the rest of the retention window.
		{FeedID: settledFeed, PageURL: "https://example.com/robots"},
		{FeedID: ourFaultFeed, PageURL: "http://169.254.169.254/latest/meta-data"},
	}}
	fetcher := &resolveFakeFetcher{
		byURL:    map[string]string{"https://example.com/ok": "https://cdn.example.com/ok.png"},
		refusals: map[string]domain.OgImageRefusal{"https://example.com/robots": domain.OgImageRefusedByRobots},
		errs: map[string]error{
			"http://169.254.169.254/latest/meta-data": errors.New("ssrf: link-local address refused"),
		},
	}

	h := resolveTestHandler(og_image_resolve_usecase.NewUsecase(store, fetcher, resolveFakeMinter{}))

	resp, err := h.ResolveOgImages(authedContext(), connect.NewRequest(&feedsv2.ResolveOgImagesRequest{
		FeedIds: []string{resolvedFeed, barredFeed, settledFeed, ourFaultFeed, neverSeenFeed},
	}))
	require.NoError(t, err)

	images := map[string]string{}
	for _, img := range resp.Msg.GetImages() {
		images[img.GetFeedId()] = img.GetOgImageProxyUrl()
	}
	unresolved := map[string]int64{}
	for _, u := range resp.Msg.GetUnresolved() {
		unresolved[u.GetFeedId()] = u.GetRetryAfterSeconds()
	}

	// 1. Resolved: in `images`, and nowhere else.
	assert.Equal(t, "/proxy?u=https://cdn.example.com/ok.png", images[resolvedFeed])
	assert.NotContains(t, unresolved, resolvedFeed)

	// 2. Asked and failed: in `unresolved` with the seconds left on the bar.
	//    Zero here would tell the client to give the card up for the session.
	require.Contains(t, unresolved, barredFeed)
	assert.Equal(t, int64(4), unresolved[barredFeed])

	// 3. Asked and settled: in `unresolved` with zero.
	require.Contains(t, unresolved, settledFeed)
	assert.Equal(t, int64(0), unresolved[settledFeed])

	// 3b. Considered and unusable: also in `unresolved` with zero. A page URL
	//     we refuse to fetch is a fault on our side, but it is settled all the
	//     same — nothing about it changes inside this window.
	require.Contains(t, unresolved, ourFaultFeed)
	assert.Equal(t, int64(0), unresolved[ourFaultFeed])
	assert.NotContains(t, images, ourFaultFeed)

	// 4. Never considered: in neither list. Only a feed the server did not look
	//    at reaches the client as silence — here, one with no row at all.
	assert.NotContains(t, images, neverSeenFeed)
	assert.NotContains(t, unresolved, neverSeenFeed)
}

// An empty request is answered with an empty response rather than by reaching
// the usecase, and both lists must be empty rather than absent-but-populated.
func TestResolveOgImages_EmptyRequest(t *testing.T) {
	h := resolveTestHandler(og_image_resolve_usecase.NewUsecase(
		&resolveFakeStore{}, &resolveFakeFetcher{}, resolveFakeMinter{}))

	resp, err := h.ResolveOgImages(authedContext(),
		connect.NewRequest(&feedsv2.ResolveOgImagesRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetImages())
	assert.Empty(t, resp.Msg.GetUnresolved())
}

// Rule 8: an unwired resolver says so rather than answering "no feed has an
// image". The guard predates the `unresolved` list and must survive it — an
// empty response with an empty unresolved list is exactly the shape a client
// reads as "every card is blank forever".
func TestResolveOgImages_UnwiredResolverIsUnimplemented(t *testing.T) {
	h := NewHandler(FeedHandlerDeps{}, &config.Config{}, slog.Default())

	_, err := h.ResolveOgImages(authedContext(), connect.NewRequest(&feedsv2.ResolveOgImagesRequest{
		FeedIds: []string{"f1"},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

// Unauthenticated callers are refused before any origin is contacted.
func TestResolveOgImages_RequiresAuth(t *testing.T) {
	h := resolveTestHandler(og_image_resolve_usecase.NewUsecase(
		&resolveFakeStore{}, &resolveFakeFetcher{}, resolveFakeMinter{}))

	_, err := h.ResolveOgImages(context.Background(),
		connect.NewRequest(&feedsv2.ResolveOgImagesRequest{FeedIds: []string{"f1"}}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}
