package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A settled answer stays settled however many times it is heard.
//
// robots.txt, "this page has no og:image" and 404 are the origin's answer, not
// a fault on the way to it. Escalating them would imply the answer might change
// with patience; it will not, and the retention window already re-opens the
// question when the row is purged.
func TestRetryAfter_SettledAnswersNeverEscalate(t *testing.T) {
	settled := []OgImageRefusal{OgImageRefusedByRobots, OgImageNoTag, OgImageRefusedNotFound}
	for _, refusal := range settled {
		for attempts := 1; attempts <= 8; attempts++ {
			assert.Zero(t, refusal.RetryAfter(attempts),
				"%s must stay permanent at attempt %d — it is an answer, not a fault", refusal, attempts)
		}
	}
}

// A 403 keeps its flat daily bar.
//
// The existing rationale is that a 403 is usually permanent policy but is also
// what a misconfigured edge returns during an incident, so one ask a day
// recovers that case for free. Escalating would break exactly the case the
// daily ask exists for, and the 7-day retention window already caps it at seven
// asks in total.
func TestRetryAfter_ForbiddenStaysDaily(t *testing.T) {
	for attempts := 1; attempts <= 8; attempts++ {
		assert.Equal(t, 24*time.Hour, OgImageRefusedForbidden.RetryAfter(attempts),
			"a 403 must not escalate: the daily ask is what recovers a misconfigured edge")
	}
}

// A transport failure is the one refusal worth asking about again soon, and the
// bar doubles so that a feed which keeps failing is asked about ever more
// rarely rather than on every scroll.
func TestRetryAfter_FetchErrorLadder(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 20 * time.Second},
		{4, 40 * time.Second},
		{8, 640 * time.Second},
		{13, 20480 * time.Second},
		// 5s << 13 is 40960s, past the ceiling.
		{14, 6 * time.Hour},
		{40, 6 * time.Hour},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, OgImageFetchError.RetryAfter(tc.attempts),
			"attempt %d", tc.attempts)
	}
}

// The first two rungs must fit under the client's own ceiling.
//
// alt-frontend-sv/src/lib/utils/ogImageRetry.ts clamps every wait to
// OG_RETRY_CEILING_MS = 10s and gives up after 3 asks. A first bar above that
// ceiling is one the browser will not wait for, so the reader's card stays
// blank for the whole session and the client's re-ask never runs even once.
// Starting at 5s buys two in-session asks before the ladder outgrows the
// browser and the row is left for the next page load to collect.
func TestRetryAfter_FirstRungsFitTheClientCeiling(t *testing.T) {
	const clientCeiling = 10 * time.Second

	require.LessOrEqual(t, OgImageFetchError.RetryAfter(1), clientCeiling,
		"the first bar must be one the browser can actually wait out")
	require.LessOrEqual(t, OgImageFetchError.RetryAfter(2), clientCeiling,
		"the second bar too, or the client gets one ask instead of three")
	require.Greater(t, OgImageFetchError.RetryAfter(3), clientCeiling,
		"by the third the ladder should outgrow the session and leave the row for the next page load")
}

// attempts is 1-based because it counts the attempt whose refusal is being
// recorded. A caller that passes the raw stored counter for a feed with no row
// yet passes 0, and that must land on the first rung rather than half of it.
func TestRetryAfter_NormalisesAttemptsBelowOne(t *testing.T) {
	assert.Equal(t, OgImageFetchError.RetryAfter(1), OgImageFetchError.RetryAfter(0))
	assert.Equal(t, OgImageFetchError.RetryAfter(1), OgImageFetchError.RetryAfter(-3))
}

// NeedsFetch is unchanged by the two new counters: they say *when* to ask
// again, never *whether* to ask now.
func TestFeedOgImageTarget_NeedsFetchIgnoresTheCounters(t *testing.T) {
	fetchable := FeedOgImageTarget{
		FeedID:            "f1",
		PageURL:           "https://example.com/a",
		Attempts:          9,
		RetryAfterSeconds: 0,
	}
	assert.True(t, fetchable.NeedsFetch(),
		"attempts spent is history; with no image and no standing bar the feed is still fetchable")

	barred := FeedOgImageTarget{
		FeedID:            "f2",
		PageURL:           "https://example.com/b",
		Suppressed:        true,
		Attempts:          1,
		RetryAfterSeconds: 5,
	}
	assert.False(t, barred.NeedsFetch())
}
