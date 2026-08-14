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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The 403 ladder is this feed's own penalty for the 403; the host's interval is
// not its unit. Multiplying the two charges the interval twice — every retry
// already waits for a host turn — and on a host an earlier 429 backed off (the
// backoff caps at an hour) it turns three retries into hours, during which the
// collector's tick is held by one publisher that answers nothing but 403.
//
// Each case gives the limiter enough burst that every attempt gets its turn
// immediately, so what these two seconds measure is the ladder alone.
func TestCollectMultipleFeeds_403LadderIsNotScaledByTheHostInterval(t *testing.T) {
	logger.InitLogger()
	allowLoopbackFeedHost(t)

	// productionHostInterval is what compose runs the feed collector at.
	const productionHostInterval = 10 * time.Second

	tests := []struct {
		name      string
		widenHost func(limiter *rate_limiter.HostRateLimiter, host string)
	}{
		{
			name:      "at the host's configured interval",
			widenHost: func(*rate_limiter.HostRateLimiter, string) {},
		},
		{
			name: "at an interval an earlier 429 widened",
			widenHost: func(limiter *rate_limiter.HostRateLimiter, host string) {
				limiter.RecordRateLimitHit(host, time.Hour) // the ceiling RecordRateLimitHit clamps to
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusForbidden)
			}))
			defer server.Close()

			feedURL, err := url.Parse(server.URL + "/feed.xml")
			require.NoError(t, err)

			limiter := rate_limiter.NewHostRateLimiter(productionHostInterval, 1+max403Retries)
			tt.widenHost(limiter, feedURL.Host)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			feedLinks := []domain.FeedLink{{ID: uuid.New(), URL: feedURL.String()}}
			_, err = CollectMultipleFeeds(ctx, feedLinks, limiter, nil)
			require.Error(t, err, "a feed that answers 403 to every attempt must fail the collection")

			assert.GreaterOrEqual(t, requests.Load(), int64(2),
				"the first 403 retry must reach the publisher inside two seconds; a ladder whose unit is the %s host interval holds it far longer than that",
				productionHostInterval)
		})
	}
}
