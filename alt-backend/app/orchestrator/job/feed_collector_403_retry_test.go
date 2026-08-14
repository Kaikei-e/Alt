package job

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"alt/domain"
	"alt/utils/logger"
	"alt/utils/rate_limiter"

	"github.com/google/uuid"
	rssFeed "github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A 403 retry is another request to the same publisher, so it has to queue for
// the same host slot the first attempt took. The retry loop used to sleep
// 1s/2s/4s of its own and go straight back into gofeed, which put four requests
// on one host inside seven seconds — below CLAUDE.md rule 2's floor and
// invisible to both the in-process bucket and the shared arbiter.
func TestCollectMultipleFeeds_403RetriesTakeAHostSlot(t *testing.T) {
	logger.InitLogger()
	allowLoopbackFeedHost(t)

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	store := &recordingSlotStore{}
	limiter := rate_limiter.NewCoordinatedHostRateLimiter(time.Millisecond, 1, rate_limiter.Coordination{
		Store:     store,
		Namespace: rate_limiter.NamespaceExternalAPI,
		Owner:     "alt-harvester/test",
	})

	feedURL, err := url.Parse(server.URL + "/feed.xml")
	require.NoError(t, err)

	feedLinks := []domain.FeedLink{{ID: uuid.New(), URL: feedURL.String()}}
	_, err = CollectMultipleFeeds(context.Background(), feedLinks, limiter, nil)
	require.Error(t, err, "a feed that answers 403 to every attempt must fail the collection")

	require.Equal(t, int64(1+max403Retries), requests.Load(),
		"the retry ladder itself is unchanged: one attempt plus max403Retries")
	assert.Len(t, store.recorded(), 1+max403Retries,
		"every attempt is a request to the publisher and must arbitrate for the host slot")
}

// With no limiter wired there is nothing else tracking the host, so the backoff
// alone has to hold rule 2's floor. The deadline here is well under that floor
// and well over the second the old ladder waited, so a retry escaping the
// interval shows up as a second call to the publisher.
func TestFetchWithRetryOn403_WithoutLimiter_HoldsTheExternalAPIFloor(t *testing.T) {
	logger.InitLogger()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	attempts := 0
	_, err := fetchWithRetryOn403(ctx, func() (*rssFeed.Feed, error) {
		attempts++
		return nil, errorWithMessage("http error: 403 Forbidden")
	}, "https://example.com/feed.xml", nil)

	require.Error(t, err, "the context runs out before the backoff does")
	assert.Equal(t, 1, attempts,
		"a 403 retry may not reach the publisher inside the 5-second external-API interval")
}
