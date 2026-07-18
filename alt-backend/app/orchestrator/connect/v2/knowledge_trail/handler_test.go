package knowledge_trail

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"alt/domain"
	knowledgetrailv1 "alt/gen/proto/alt/knowledge_trail/v1"
	"alt/mocks"
	"alt/orchestrator/usecase/get_knowledge_trail_usecase"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/orchestrator/usecase/search_trail_usecase"
)

type fakeTrailPort struct {
	footprints []domain.TrailFootprint
	episodes   []domain.TrailEpisode
}

func (f *fakeTrailPort) GetTrailFootprints(_ context.Context, _ uuid.UUID, _ string, _ int, _ []string) ([]domain.TrailFootprint, []domain.TrailBranch, []domain.TrailEpisode, string, bool, error) {
	return f.footprints, nil, f.episodes, "", false, nil
}

type fakeThumbnailPort struct {
	urls map[string]string
}

func (f *fakeThumbnailPort) GetOgImageURLsByArticleIDs(_ context.Context, _ []string) (map[string]string, error) {
	return f.urls, nil
}

type fakeSearchPort struct {
	hits []domain.SearchIndexerArticleHit
}

func (f *fakeSearchPort) SearchArticles(_ context.Context, _ string, _ string) ([]domain.SearchIndexerArticleHit, error) {
	return f.hits, nil
}

func (f *fakeSearchPort) SearchArticlesWithPagination(_ context.Context, _ string, _ string, _ int, _ int) ([]domain.SearchIndexerArticleHit, int64, error) {
	return nil, 0, nil
}

func (f *fakeSearchPort) SearchRecapsByTag(_ context.Context, _ string, _ int) ([]*domain.RecapSearchResult, error) {
	return nil, nil
}

func (f *fakeSearchPort) SearchRecapsByQuery(_ context.Context, _ string, _ int) ([]*domain.RecapSearchResult, int64, error) {
	return nil, 0, nil
}

type fakeSearchTrailPort struct {
	episodes []domain.TrailEpisode
}

func (f *fakeSearchTrailPort) SearchTrailFootprints(_ context.Context, _ uuid.UUID, _ []string, _ int) ([]domain.TrailEpisode, error) {
	return f.episodes, nil
}

func userCtx() context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TenantID:  uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Email:     "trail@example.com",
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

// A collapsed footprint's contact count and first contact time must survive
// the BFF mapping — dropping either silently regresses the spine back to the
// one-row-per-day duplicate display (D24).
func TestGetTrail_MapsCollapsedContactFields(t *testing.T) {
	port := &fakeTrailPort{footprints: []domain.TrailFootprint{{
		FootprintKey:    "open:article:1",
		Verb:            "read",
		ItemKey:         "article:1",
		Title:           "US military courts in the UK",
		OccurredAt:      time.Date(2026, 7, 7, 22, 20, 0, 0, time.UTC),
		FirstOccurredAt: time.Date(2026, 6, 27, 18, 37, 0, 0, time.UTC),
		ContactCount:    2,
		Wear:            "worn",
	}}}
	h := NewHandler(get_knowledge_trail_usecase.NewGetKnowledgeTrailUsecase(port, &fakeThumbnailPort{}), nil, nil, nil, nil, slog.Default())

	resp, err := h.GetTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.GetTrailRequest{Limit: 20}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Footprints, 1)

	fp := resp.Msg.Footprints[0]
	assert.Equal(t, int32(2), fp.ContactCount)
	assert.Equal(t, "2026-06-27T18:37:00Z", fp.FirstOccurredAt)
	assert.Equal(t, "2026-07-07T22:20:00Z", fp.OccurredAt)
}

// A single-contact footprint still reports itself honestly: count 1 and a
// first contact equal to the latest.
func TestGetTrail_SingleContactKeepsCountOne(t *testing.T) {
	at := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	port := &fakeTrailPort{footprints: []domain.TrailFootprint{{
		FootprintKey:    "open:article:2",
		Verb:            "read",
		ItemKey:         "article:2",
		OccurredAt:      at,
		FirstOccurredAt: at,
		ContactCount:    1,
	}}}
	h := NewHandler(get_knowledge_trail_usecase.NewGetKnowledgeTrailUsecase(port, &fakeThumbnailPort{}), nil, nil, nil, nil, slog.Default())

	resp, err := h.GetTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.GetTrailRequest{Limit: 20}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Footprints, 1)
	assert.Equal(t, int32(1), resp.Msg.Footprints[0].ContactCount)
	assert.Equal(t, resp.Msg.Footprints[0].OccurredAt, resp.Msg.Footprints[0].FirstOccurredAt)
}

// Episodes (D24/D30, Wave 8) map through to the wire, keyed/ordered exactly
// as the usecase returns them.
func TestGetTrail_MapsEpisodes(t *testing.T) {
	port := &fakeTrailPort{episodes: []domain.TrailEpisode{{
		EpisodeKey: "ep:open:article:1",
		Wear:       "deep",
		Footprints: []domain.TrailFootprint{{
			FootprintKey: "open:article:1",
			ItemKey:      "article:1",
			Verb:         "read",
		}},
	}}}
	h := NewHandler(get_knowledge_trail_usecase.NewGetKnowledgeTrailUsecase(port, &fakeThumbnailPort{}), nil, nil, nil, nil, slog.Default())

	resp, err := h.GetTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.GetTrailRequest{Limit: 20}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Episodes, 1)
	ep := resp.Msg.Episodes[0]
	assert.Equal(t, "ep:open:article:1", ep.EpisodeKey)
	assert.Equal(t, "deep", ep.Wear)
	require.Len(t, ep.Footprints, 1)
	assert.Equal(t, "article:1", ep.Footprints[0].ItemKey)
	assert.Empty(t, ep.ThumbnailUrl, "no OG image was resolved, so the card must degrade to text")
}

// D29: a resolved OG image is signed through the existing image-proxy signer
// (mirrors feeds.enrichWithProxyURLs) before it reaches the wire.
func TestGetTrail_SignsEpisodeThumbnailWhenRawURLPresent(t *testing.T) {
	articleID := uuid.New().String()
	rawURL := "https://example.com/a.png"
	signedURL := "https://cdn.example.com/proxy/signed-a"

	port := &fakeTrailPort{episodes: []domain.TrailEpisode{{
		EpisodeKey: "ep:open:article:1",
		Footprints: []domain.TrailFootprint{{FootprintKey: "open:article:1", ItemKey: "article:" + articleID}},
	}}}
	thumbs := &fakeThumbnailPort{urls: map[string]string{articleID: rawURL}}

	ctrl := gomock.NewController(t)
	signer := mocks.NewMockImageProxySignerPort(ctrl)
	signer.EXPECT().GenerateProxyURL(rawURL).Return(signedURL)
	imageProxy := image_proxy_usecase.NewImageProxyUsecase(nil, nil, nil, signer, nil, nil, 0, 0, 0)

	h := NewHandler(get_knowledge_trail_usecase.NewGetKnowledgeTrailUsecase(port, thumbs), nil, nil, nil, imageProxy, slog.Default())

	resp, err := h.GetTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.GetTrailRequest{Limit: 20}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Episodes, 1)
	assert.Equal(t, signedURL, resp.Msg.Episodes[0].ThumbnailUrl)
}

// A raw thumbnail URL must never reach the wire unsigned: with no image
// proxy wired, the card degrades to text rather than leaking the raw URL.
func TestGetTrail_NoImageProxyLeavesThumbnailEmpty(t *testing.T) {
	articleID := uuid.New().String()
	port := &fakeTrailPort{episodes: []domain.TrailEpisode{{
		EpisodeKey: "ep:open:article:1",
		Footprints: []domain.TrailFootprint{{FootprintKey: "open:article:1", ItemKey: "article:" + articleID}},
	}}}
	thumbs := &fakeThumbnailPort{urls: map[string]string{articleID: "https://example.com/a.png"}}

	h := NewHandler(get_knowledge_trail_usecase.NewGetKnowledgeTrailUsecase(port, thumbs), nil, nil, nil, nil, slog.Default())

	resp, err := h.GetTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.GetTrailRequest{Limit: 20}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Episodes, 1)
	assert.Empty(t, resp.Msg.Episodes[0].ThumbnailUrl)
}

// SearchTrail (Wave 9, D25) maps the usecase's episodes and matched item keys
// through to the wire, reusing the same episode/footprint mapping GetTrail
// uses (h.mapEpisodes).
func TestSearchTrail_MapsEpisodesAndMatchedItemKeys(t *testing.T) {
	articleID := uuid.New().String()
	searchPort := &fakeSearchPort{hits: []domain.SearchIndexerArticleHit{{ID: articleID}}}
	trailPort := &fakeSearchTrailPort{episodes: []domain.TrailEpisode{{
		EpisodeKey: "ep:open:article:1",
		Wear:       "worn",
		Footprints: []domain.TrailFootprint{{FootprintKey: "open:article:1", ItemKey: "article:" + articleID, Verb: "read"}},
	}}}
	searchUC := search_trail_usecase.NewSearchTrailUsecase(searchPort, trailPort, &fakeThumbnailPort{})
	h := NewHandler(nil, nil, nil, searchUC, nil, slog.Default())

	resp, err := h.SearchTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.SearchTrailRequest{Query: "llm", Limit: 20}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Episodes, 1)
	assert.Equal(t, "ep:open:article:1", resp.Msg.Episodes[0].EpisodeKey)
	assert.Equal(t, "worn", resp.Msg.Episodes[0].Wear)
	require.Len(t, resp.Msg.Episodes[0].Footprints, 1)
	assert.Equal(t, "article:"+articleID, resp.Msg.Episodes[0].Footprints[0].ItemKey)
	assert.Equal(t, []string{"article:" + articleID}, resp.Msg.MatchedItemKeys)
}

// D29: an OG image resolved for the episode's representative article must be
// signed through the image-proxy signer before it reaches the wire — the
// same signing path GetTrail uses (h.signThumbnail), reused rather than
// duplicated.
func TestSearchTrail_SignsEpisodeThumbnailWhenRawURLPresent(t *testing.T) {
	articleID := uuid.New().String()
	rawURL := "https://example.com/a.png"
	signedURL := "https://cdn.example.com/proxy/signed-a"

	searchPort := &fakeSearchPort{hits: []domain.SearchIndexerArticleHit{{ID: articleID}}}
	trailPort := &fakeSearchTrailPort{episodes: []domain.TrailEpisode{{
		EpisodeKey: "ep:1",
		Footprints: []domain.TrailFootprint{{FootprintKey: "fp:1", ItemKey: "article:" + articleID}},
	}}}
	thumbs := &fakeThumbnailPort{urls: map[string]string{articleID: rawURL}}
	searchUC := search_trail_usecase.NewSearchTrailUsecase(searchPort, trailPort, thumbs)

	ctrl := gomock.NewController(t)
	signer := mocks.NewMockImageProxySignerPort(ctrl)
	signer.EXPECT().GenerateProxyURL(rawURL).Return(signedURL)
	imageProxy := image_proxy_usecase.NewImageProxyUsecase(nil, nil, nil, signer, nil, nil, 0, 0, 0)

	h := NewHandler(nil, nil, nil, searchUC, imageProxy, slog.Default())

	resp, err := h.SearchTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.SearchTrailRequest{Query: "llm", Limit: 20}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Episodes, 1)
	assert.Equal(t, signedURL, resp.Msg.Episodes[0].ThumbnailUrl)
}

// Unauthenticated requests are rejected before the usecase is ever reached.
func TestSearchTrail_UnauthenticatedReturnsUnauthenticated(t *testing.T) {
	searchUC := search_trail_usecase.NewSearchTrailUsecase(&fakeSearchPort{}, &fakeSearchTrailPort{}, &fakeThumbnailPort{})
	h := NewHandler(nil, nil, nil, searchUC, nil, slog.Default())

	_, err := h.SearchTrail(context.Background(), connect.NewRequest(&knowledgetrailv1.SearchTrailRequest{Query: "llm"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// An empty query is a structurally invalid request — the usecase's
// ErrInvalidRequest maps to CodeInvalidArgument, not CodeInternal.
func TestSearchTrail_EmptyQueryReturnsInvalidArgument(t *testing.T) {
	searchUC := search_trail_usecase.NewSearchTrailUsecase(&fakeSearchPort{}, &fakeSearchTrailPort{}, &fakeThumbnailPort{})
	h := NewHandler(nil, nil, nil, searchUC, nil, slog.Default())

	_, err := h.SearchTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.SearchTrailRequest{Query: "   "}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// Rule 8 (no silent fallback): an unwired search usecase must panic rather
// than silently return an empty result, mirroring EmitTrailOutcome's guard —
// a DI gap must be loud, not indistinguishable from "zero results".
func TestSearchTrail_UnwiredUsecasePanics(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, slog.Default())

	assert.Panics(t, func() {
		_, _ = h.SearchTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.SearchTrailRequest{Query: "llm"}))
	})
}
