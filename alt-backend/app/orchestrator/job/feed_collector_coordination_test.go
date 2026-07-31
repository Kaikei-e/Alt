package job

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"alt/utils/rate_limiter"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingSlotStore stands in for Redis and records the keys it was asked for.
type recordingSlotStore struct {
	mu   sync.Mutex
	keys []string
}

func (r *recordingSlotStore) AcquireSlot(_ context.Context, key, _ string, _ time.Duration) (bool, time.Duration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys = append(r.keys, key)
	return true, 0, nil
}

func (r *recordingSlotStore) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.keys...)
}

const minimalRSS = `<?xml version="1.0"?><rss version="2.0"><channel><title>t</title><link>http://x</link><description>d</description></channel></rss>`

// The hourly collector is one half of the duplicate the ADR-000954 review
// found: it polls the same publisher cmd/backend polls when a user registers a
// feed. It used to build its own 5-second limiter inside CollectFeedsJob,
// which no composition root could reach — so no arbiter could be attached to
// it and the job was structurally incapable of coordinating with anything.
func TestCollectSingleFeed_ConsultsTheSharedSlot(t *testing.T) {
	// httptest binds to loopback, which the SSRF guard blocks unless the
	// operator allow-listed it (same pattern as feed_body_limit_test.go).
	t.Setenv("FEED_ALLOWED_HOSTS", "127.0.0.1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(minimalRSS))
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

	_, err = CollectSingleFeed(context.Background(), *feedURL, limiter)
	require.NoError(t, err)

	keys := store.recorded()
	require.Len(t, keys, 1)
	assert.True(t, strings.HasPrefix(keys[0], rate_limiter.NamespaceExternalAPI+":"),
		"feed polling must arbitrate in the external_api class, got %q", keys[0])
	assert.True(t, strings.HasSuffix(keys[0], feedURL.Host),
		"the slot is per host, got %q", keys[0])
}

// CollectFeedsJob must take its limiter from the composition root. A limiter
// it constructs itself cannot be coordinated, and the job would silently keep
// the per-process guarantee while the startup log said "distributed".
func TestCollectFeedsJob_TakesTheInjectedLimiter(t *testing.T) {
	store := &recordingSlotStore{}
	limiter := rate_limiter.NewCoordinatedHostRateLimiter(time.Millisecond, 1, rate_limiter.Coordination{
		Store:     store,
		Namespace: rate_limiter.NamespaceExternalAPI,
		Owner:     "alt-harvester/test",
	})

	fn := CollectFeedsJob(nil, limiter)
	require.NotNil(t, fn)
}
