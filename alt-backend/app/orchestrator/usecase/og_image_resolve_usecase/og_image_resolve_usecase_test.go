package og_image_resolve_usecase

import (
	"context"
	"errors"
	"testing"

	"alt/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	targets []domain.FeedOgImageTarget
	saved   []savedCall
	err     error
}

type savedCall struct {
	feedID   string
	imageURL string
	refusal  domain.OgImageRefusal
}

func (f *fakeStore) FetchFeedOgImageTargets(_ context.Context, _ []string) ([]domain.FeedOgImageTarget, error) {
	return f.targets, f.err
}

func (f *fakeStore) SaveFeedOgImage(_ context.Context, feedID, imageURL string, refusal domain.OgImageRefusal) error {
	f.saved = append(f.saved, savedCall{feedID, imageURL, refusal})
	return nil
}

type fakeFetcher struct {
	byURL    map[string]string
	refusals map[string]domain.OgImageRefusal
	calls    []string
}

func (f *fakeFetcher) FetchOgImage(_ context.Context, pageURL string) (string, domain.OgImageRefusal, error) {
	f.calls = append(f.calls, pageURL)
	if r, ok := f.refusals[pageURL]; ok {
		return "", r, nil
	}
	return f.byURL[pageURL], "", nil
}

type fakeMinter struct{ warmed []string }

func (m *fakeMinter) GenerateProxyURL(imageURL string) string { return "/proxy/" + imageURL }
func (m *fakeMinter) WarmCache(_ context.Context, imageURL string) {
	m.warmed = append(m.warmed, imageURL)
}

func newUsecase(store *fakeStore, fetcher *fakeFetcher, minter *fakeMinter) *Usecase {
	return NewUsecase(store, fetcher, minter)
}

// The core case: a feed with nothing known gets one origin request, the result
// is recorded, and the caller receives a proxy URL it can render.
func TestExecute_ResolvesAndRecords(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "https://example.com/a"},
	}}
	fetcher := &fakeFetcher{byURL: map[string]string{"https://example.com/a": "https://cdn.example.com/a.png"}}
	minter := &fakeMinter{}

	got, err := newUsecase(store, fetcher, minter).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"f1": "/proxy/https://cdn.example.com/a.png"}, got)
	require.Len(t, store.saved, 1)
	assert.Equal(t, savedCall{"f1", "https://cdn.example.com/a.png", ""}, store.saved[0])
	assert.Equal(t, []string{"https://example.com/a"}, fetcher.calls)
	assert.Equal(t, []string{"https://cdn.example.com/a.png"}, minter.warmed,
		"the image the reader is about to request should be warmed while we are already here")
}

// A feed we already hold an image for must not cost the origin a request. This
// is the difference between resolving on demand and crawling on scroll.
func TestExecute_AlreadyHeldImageIsNotFetched(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "https://example.com/a", OgImageURL: "https://cdn.example.com/known.png"},
	}}
	fetcher := &fakeFetcher{}
	minter := &fakeMinter{}

	got, err := newUsecase(store, fetcher, minter).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"f1": "/proxy/https://cdn.example.com/known.png"}, got)
	assert.Empty(t, fetcher.calls, "an image we already hold must not be re-fetched")
	assert.Empty(t, store.saved, "nothing changed, so nothing should be written")
}

// A standing refusal must suppress the request entirely. Without this, every
// scroll past a card whose origin said no becomes another request to that
// origin — the behaviour that got the batch job blocked in the first place.
func TestExecute_SuppressedFeedIsNotFetched(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "https://example.com/a", Suppressed: true},
	}}
	fetcher := &fakeFetcher{}

	got, err := newUsecase(store, fetcher, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Empty(t, got, "a suppressed feed yields no URL")
	assert.Empty(t, fetcher.calls)
	assert.Empty(t, store.saved, "the refusal is already recorded; re-writing it would reset its age")
}

// A refusal is recorded, and the feed is absent from the response rather than
// present with an empty URL.
func TestExecute_RefusalIsRecordedAndOmitted(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "https://example.com/a"},
	}}
	fetcher := &fakeFetcher{refusals: map[string]domain.OgImageRefusal{
		"https://example.com/a": domain.OgImageRefusedByRobots,
	}}

	got, err := newUsecase(store, fetcher, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Empty(t, got)
	require.Len(t, store.saved, 1)
	assert.Equal(t, savedCall{"f1", "", domain.OgImageRefusedByRobots}, store.saved[0])
}

// One refusing feed must not stop the others in the same viewport resolving.
func TestExecute_OneRefusalDoesNotBlockTheBatch(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "https://example.com/a"},
		{FeedID: "f2", PageURL: "https://example.com/b"},
	}}
	fetcher := &fakeFetcher{
		byURL:    map[string]string{"https://example.com/b": "https://cdn.example.com/b.png"},
		refusals: map[string]domain.OgImageRefusal{"https://example.com/a": domain.OgImageRefusedForbidden},
	}

	got, err := newUsecase(store, fetcher, &fakeMinter{}).Execute(context.Background(), []string{"f1", "f2"})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"f2": "/proxy/https://cdn.example.com/b.png"}, got)
	require.Len(t, store.saved, 2)
}

// An empty request must not reach the store at all.
func TestExecute_NoIDs(t *testing.T) {
	store := &fakeStore{}
	got, err := newUsecase(store, &fakeFetcher{}, &fakeMinter{}).Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// The batch is capped, because the cap is a bound on how many publishers one
// viewport change can cause us to contact.
func TestExecute_CapsTheBatch(t *testing.T) {
	ids := make([]string, MaxBatch+5)
	for i := range ids {
		ids[i] = "f"
	}

	store := &fakeStore{}
	_, err := newUsecase(store, &fakeFetcher{}, &fakeMinter{}).Execute(context.Background(), ids)
	require.NoError(t, err)
}

// A store failure is an error the caller should see, not an empty result that
// looks like "no feed has an image".
func TestExecute_StoreFailurePropagates(t *testing.T) {
	store := &fakeStore{err: errors.New("datahub unreachable")}
	_, err := newUsecase(store, &fakeFetcher{}, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
	require.Error(t, err)
}
