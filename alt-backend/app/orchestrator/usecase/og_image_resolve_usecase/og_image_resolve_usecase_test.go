package og_image_resolve_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

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
	feedID     string
	imageURL   string
	refusal    domain.OgImageRefusal
	retryAfter time.Duration
}

func (f *fakeStore) FetchFeedOgImageTargets(_ context.Context, _ []string) ([]domain.FeedOgImageTarget, error) {
	return f.targets, f.err
}

func (f *fakeStore) SaveFeedOgImage(
	_ context.Context,
	feedID, imageURL string,
	refusal domain.OgImageRefusal,
	retryAfter time.Duration,
) error {
	f.saved = append(f.saved, savedCall{feedID, imageURL, refusal, retryAfter})
	return nil
}

type fakeFetcher struct {
	byURL    map[string]string
	refusals map[string]domain.OgImageRefusal
	errs     map[string]error
	calls    []string
}

func (f *fakeFetcher) FetchOgImage(_ context.Context, pageURL string) (string, domain.OgImageRefusal, error) {
	f.calls = append(f.calls, pageURL)
	if err, ok := f.errs[pageURL]; ok {
		return "", "", err
	}
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

	got, unresolved, err := newUsecase(store, fetcher, minter).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"f1": "/proxy/https://cdn.example.com/a.png"}, got)
	assert.Empty(t, unresolved, "a feed that resolved belongs in one list only")
	require.Len(t, store.saved, 1)
	assert.Equal(t, savedCall{"f1", "https://cdn.example.com/a.png", "", 0}, store.saved[0])
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

	got, unresolved, err := newUsecase(store, fetcher, minter).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"f1": "/proxy/https://cdn.example.com/known.png"}, got)
	assert.Empty(t, unresolved)
	assert.Empty(t, fetcher.calls, "an image we already hold must not be re-fetched")
	assert.Empty(t, store.saved, "nothing changed, so nothing should be written")
}

// A standing refusal must suppress the request entirely. Without this, every
// scroll past a card whose origin said no becomes another request to that
// origin — the behaviour that got the batch job blocked in the first place.
//
// It must still be reported, and with the seconds left on the bar rather than
// with the zero that means "permanent". From the second ask onwards this is the
// branch every failing feed takes, so answering zero here is what would make a
// five-second bar look to the reader's client like a settled refusal.
func TestExecute_SuppressedFeedIsReportedWithItsRemainingBar(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "https://example.com/a", Suppressed: true, Attempts: 1, RetryAfterSeconds: 4},
	}}
	fetcher := &fakeFetcher{}

	got, unresolved, err := newUsecase(store, fetcher, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Empty(t, got, "a suppressed feed yields no URL")
	assert.Equal(t, map[string]int64{"f1": 4}, unresolved,
		"the client can only come back at the right moment if it is told when that is")
	assert.Empty(t, fetcher.calls)
	assert.Empty(t, store.saved, "the refusal is already recorded; re-writing it would reset its age")
}

// A refusal whose bar is permanent within this window reports zero, which is
// the wire form of "asking again buys nothing".
func TestExecute_SuppressedPermanentlyReportsZero(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "https://example.com/a", Suppressed: true, Attempts: 1, RetryAfterSeconds: 0},
	}}

	_, unresolved, err := newUsecase(store, &fakeFetcher{}, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{"f1": 0}, unresolved)
}

// A feed with no page URL can never be resolved inside this window, and saying
// so is more use to the client than silence: absence would invite it to ask
// again on the next page load, forever, for a question that has no answer.
func TestExecute_FeedWithNoPageURLIsReportedAsSettled(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: ""},
	}}
	fetcher := &fakeFetcher{}

	got, unresolved, err := newUsecase(store, fetcher, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Empty(t, got)
	assert.Equal(t, map[string]int64{"f1": 0}, unresolved)
	assert.Empty(t, fetcher.calls)
}

// A refusal is recorded, and the feed is absent from the resolved map rather
// than present with an empty URL.
func TestExecute_RefusalIsRecordedAndOmitted(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "https://example.com/a"},
	}}
	fetcher := &fakeFetcher{refusals: map[string]domain.OgImageRefusal{
		"https://example.com/a": domain.OgImageRefusedByRobots,
	}}

	got, unresolved, err := newUsecase(store, fetcher, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Empty(t, got)
	assert.Equal(t, map[string]int64{"f1": 0}, unresolved,
		"a robots.txt disallow is settled inside this window whatever the attempt count")
	require.Len(t, store.saved, 1)
	assert.Equal(t, savedCall{"f1", "", domain.OgImageRefusedByRobots, 0}, store.saved[0])
}

// The bar written to the store and the bar reported to the client are the same
// number, and it is the rung the attempt just spent earned.
//
// A target carrying Attempts=1 has already failed once, so the attempt being
// recorded here is the second: 5s doubled to 10s. Getting this off by one would
// desynchronise the row the next reader reads from the answer this reader got.
func TestExecute_FetchErrorBarEscalatesWithAttempts(t *testing.T) {
	cases := []struct {
		priorAttempts int
		want          time.Duration
	}{
		{0, 5 * time.Second},
		{1, 10 * time.Second},
		{2, 20 * time.Second},
	}

	for _, tc := range cases {
		store := &fakeStore{targets: []domain.FeedOgImageTarget{
			{FeedID: "f1", PageURL: "https://example.com/a", Attempts: tc.priorAttempts},
		}}
		fetcher := &fakeFetcher{refusals: map[string]domain.OgImageRefusal{
			"https://example.com/a": domain.OgImageFetchError,
		}}

		_, unresolved, err := newUsecase(store, fetcher, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
		require.NoError(t, err)

		require.Len(t, store.saved, 1)
		assert.Equal(t, tc.want, store.saved[0].retryAfter,
			"prior attempts %d", tc.priorAttempts)
		assert.Equal(t, map[string]int64{"f1": int64(tc.want.Seconds())}, unresolved,
			"the number stored and the number reported must not drift apart")
	}
}

// A malformed URL or an SSRF rejection is a feed we considered and could not
// resolve, so it goes into `unresolved` with a zero — settled for this window.
// The wire answer and the stored row are separate questions: no refusal is
// written, because the fault is ours and a row would suppress the feed for
// every later reader, but this reader is still told not to come back. A page
// URL we refuse to fetch will be refused identically on the next ask, and
// answering with silence would have the client putting that question forever.
func TestExecute_FetchErrorOnOurSideIsUnresolvedAndSettled(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "http://169.254.169.254/latest/meta-data"},
	}}
	fetcher := &fakeFetcher{errs: map[string]error{
		"http://169.254.169.254/latest/meta-data": errors.New("ssrf: link-local address refused"),
	}}

	got, unresolved, err := newUsecase(store, fetcher, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
	require.NoError(t, err)

	assert.Empty(t, got)
	assert.Equal(t, map[string]int64{"f1": 0}, unresolved,
		"a feed the server considered must not be answered with the silence that means 'never looked'")
	assert.Empty(t, store.saved,
		"a fault on our side is not the origin's answer, so it is not recorded as a refusal")
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

	got, unresolved, err := newUsecase(store, fetcher, &fakeMinter{}).Execute(context.Background(), []string{"f1", "f2"})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"f2": "/proxy/https://cdn.example.com/b.png"}, got)
	assert.Equal(t, map[string]int64{"f1": int64((24 * time.Hour).Seconds())}, unresolved)
	require.Len(t, store.saved, 2)
}

// A feed id with no row is in neither list. Absence is how "we never reached
// this feed" is said, and it is the only outcome that has no wire symbol.
func TestExecute_FeedWithNoRowIsInNeitherList(t *testing.T) {
	store := &fakeStore{targets: []domain.FeedOgImageTarget{
		{FeedID: "f1", PageURL: "https://example.com/a"},
	}}
	fetcher := &fakeFetcher{byURL: map[string]string{"https://example.com/a": "https://cdn.example.com/a.png"}}

	got, unresolved, err := newUsecase(store, fetcher, &fakeMinter{}).
		Execute(context.Background(), []string{"f1", "f-no-row"})
	require.NoError(t, err)

	assert.NotContains(t, got, "f-no-row")
	assert.NotContains(t, unresolved, "f-no-row")
}

// An empty request must not reach the store at all.
func TestExecute_NoIDs(t *testing.T) {
	store := &fakeStore{}
	got, unresolved, err := newUsecase(store, &fakeFetcher{}, &fakeMinter{}).Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Empty(t, unresolved)
}

// The batch is capped, because the cap is a bound on how many publishers one
// viewport change can cause us to contact. The feeds the cap cut are in neither
// list: no origin was spent on them, so the client may ask again at once.
func TestExecute_CapsTheBatch(t *testing.T) {
	ids := make([]string, MaxBatch+5)
	for i := range ids {
		ids[i] = "f"
	}

	store := &fakeStore{}
	_, unresolved, err := newUsecase(store, &fakeFetcher{}, &fakeMinter{}).Execute(context.Background(), ids)
	require.NoError(t, err)
	assert.Empty(t, unresolved, "a feed the cap trimmed was never considered, so it is reported as neither")
}

// A store failure is an error the caller should see, not an empty result that
// looks like "no feed has an image".
func TestExecute_StoreFailurePropagates(t *testing.T) {
	store := &fakeStore{err: errors.New("datahub unreachable")}
	_, _, err := newUsecase(store, &fakeFetcher{}, &fakeMinter{}).Execute(context.Background(), []string{"f1"})
	require.Error(t, err)
}
