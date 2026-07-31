package datahub

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureHandler collects the slog records written through it.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (c *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (c *captureHandler) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r.Clone())
	return nil
}

func (c *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *captureHandler) WithGroup(string) slog.Handler      { return c }

func (c *captureHandler) messages(msg string) []slog.Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []slog.Record
	for _, r := range c.records {
		if r.Message == msg {
			out = append(out, r)
		}
	}
	return out
}

func attr(t *testing.T, r slog.Record, key string) slog.Value {
	t.Helper()
	var found slog.Value
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found, ok = a.Value, true
			return false
		}
		return true
	})
	require.Truef(t, ok, "record %q has no attribute %q", r.Message, key)
	return found
}

// fakeClock hands out a time the test moves by hand.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func callN(notice *legacyNamespaceNotice, procedure string, n int) {
	next := connect.UnaryFunc(func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil //nolint:nilnil // the interceptor under test ignores the response
	})
	wrapped := notice.interceptor()(next)
	for range n {
		req := &procedureRequest{AnyRequest: connect.NewRequest(&struct{}{}), procedure: procedure}
		_, _ = wrapped(context.Background(), req)
	}
}

// procedureRequest overrides Spec().Procedure, which connect.NewRequest leaves
// empty outside a real client or handler stack.
type procedureRequest struct {
	connect.AnyRequest
	procedure string
}

func (r *procedureRequest) Spec() connect.Spec {
	return connect.Spec{Procedure: r.procedure}
}

// The point of rate limiting is that a peer which has not migrated yet must
// not be able to fill the log. search-indexer alone polls
// BackendInternalService on a loop; one line per request would bury every
// other line alt-data-hub writes, and an operator who filters it out stops
// seeing the migration signal at all.
func TestLegacyNamespaceNotice_LogsOncePerIntervalPerProcedure(t *testing.T) {
	const interval = 5 * time.Minute

	tests := []struct {
		name     string
		steps    func(*legacyNamespaceNotice, *fakeClock)
		wantLogs int
		// wantSuppressed is the calls_since_last_log value of the final line.
		wantSuppressed int64
	}{
		{
			name: "the first call always logs",
			steps: func(n *legacyNamespaceNotice, _ *fakeClock) {
				callN(n, "/services.backend.v1.BackendInternalService/GetArticleByID", 1)
			},
			wantLogs:       1,
			wantSuppressed: 1,
		},
		{
			name: "a burst inside one interval logs once",
			steps: func(n *legacyNamespaceNotice, _ *fakeClock) {
				callN(n, "/services.backend.v1.BackendInternalService/GetArticleByID", 500)
			},
			wantLogs:       1,
			wantSuppressed: 1,
		},
		{
			name: "the next interval logs again and reports the calls it swallowed",
			steps: func(n *legacyNamespaceNotice, clk *fakeClock) {
				callN(n, "/services.backend.v1.BackendInternalService/GetArticleByID", 100)
				clk.advance(interval + time.Second)
				callN(n, "/services.backend.v1.BackendInternalService/GetArticleByID", 1)
			},
			wantLogs:       2,
			wantSuppressed: 100,
		},
		{
			name: "each procedure carries its own budget",
			steps: func(n *legacyNamespaceNotice, _ *fakeClock) {
				callN(n, "/services.backend.v1.BackendInternalService/GetArticleByID", 10)
				callN(n, "/services.backend.v1.BackendInternalService/CreateArticle", 10)
			},
			wantLogs:       2,
			wantSuppressed: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := &captureHandler{}
			clk := &fakeClock{now: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)}
			notice := newLegacyNamespaceNotice(slog.New(cap), interval, clk.Now)

			tt.steps(notice, clk)

			logs := cap.messages(legacyNamespaceCalledMsg)
			require.Len(t, logs, tt.wantLogs)

			last := logs[len(logs)-1]
			assert.Equal(t, tt.wantSuppressed, attr(t, last, "calls_since_last_log").Int64())
			assert.Equal(t, "alt.datahub.v1.DataHubService", attr(t, last, "replacement").String())
		})
	}
}

// The interceptor observes; it must not change what the caller receives.
func TestLegacyNamespaceNotice_PassesResponsesAndErrorsThrough(t *testing.T) {
	notice := newLegacyNamespaceNotice(slog.New(&captureHandler{}), time.Minute, time.Now)

	wantErr := connect.NewError(connect.CodeNotFound, errors.New("nope"))
	wrapped := notice.interceptor()(connect.UnaryFunc(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, wantErr
	}))

	req := connect.NewRequest(&struct{}{})
	_, err := wrapped(context.Background(), &procedureRequest{
		AnyRequest: req,
		procedure:  "/services.backend.v1.BackendInternalService/GetArticleByID",
	})
	assert.ErrorIs(t, err, wantErr)
}
