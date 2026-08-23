package mqhub_connect

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	mqhubv1 "alt/gen/proto/services/mqhub/v1"
	"alt/gen/proto/services/mqhub/v1/mqhubv1connect"
)

// capturingMQHubClient is a fake MQHubServiceClient that records the events the
// producer hands to Publish, so tests can assert on the envelope the real
// PublishArticleCreated/Updated/SummarizeRequested/IndexArticle methods build --
// including the Metadata map, which is the field the search-indexer /
// pre-processor consumer pacts require to carry a trace_id.
//
// The embedded interface supplies nil implementations for the RPCs these tests
// never call; only Publish is overridden.
type capturingMQHubClient struct {
	mqhubv1connect.MQHubServiceClient
	events []*mqhubv1.Event
}

func (c *capturingMQHubClient) Publish(_ context.Context, req *connect.Request[mqhubv1.PublishRequest]) (*connect.Response[mqhubv1.PublishResponse], error) {
	c.events = append(c.events, req.Msg.GetEvent())
	return connect.NewResponse(&mqhubv1.PublishResponse{MessageId: "msg-test-1"}), nil
}

// validSpanContext returns a context carrying a valid, sampled OTel span context
// with a fixed TraceID/SpanID so tests can assert the exact hex strings the
// producer must inject into event Metadata.
func validSpanContext(t *testing.T) (context.Context, trace.SpanContext) {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	require.NoError(t, err)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	require.True(t, sc.IsValid(), "constructed span context must be valid")
	return trace.ContextWithSpanContext(context.Background(), sc), sc
}

// TestPublishInjectsTraceIDFromSpanContext is the CDC RED for CDC-1.
//
// The search-indexer / pre-processor consumer pacts require $.metadata.trace_id
// on every ArticleCreated/ArticleUpdated/SummarizeRequested/IndexArticle event.
// Before the fix the producer sent Metadata: map[string]string{} (empty), so
// this asserts the real drift: with a valid span on the context, the envelope
// the producer publishes must carry metadata["trace_id"] == span TraceID hex
// (and span_id == span SpanID hex). Empty Metadata fails these assertions.
func TestPublishInjectsTraceIDFromSpanContext(t *testing.T) {
	cases := []struct {
		name    string
		publish func(ctx context.Context, c *Client) (string, error)
	}{
		{
			name: "PublishArticleCreated",
			publish: func(ctx context.Context, c *Client) (string, error) {
				return c.PublishArticleCreated(ctx, ArticleCreatedPayload{ArticleID: "art-1", UserID: "user-1", FeedID: "feed-1", Title: "t", URL: "https://example.com/a"})
			},
		},
		{
			name: "PublishArticleUpdated",
			publish: func(ctx context.Context, c *Client) (string, error) {
				return c.PublishArticleUpdated(ctx, ArticleCreatedPayload{ArticleID: "art-1", UserID: "user-1", FeedID: "feed-1", Title: "t", URL: "https://example.com/a"})
			},
		},
		{
			name: "PublishSummarizeRequested",
			publish: func(ctx context.Context, c *Client) (string, error) {
				return c.PublishSummarizeRequested(ctx, SummarizeRequestedPayload{ArticleID: "art-1", UserID: "user-1", Title: "t"})
			},
		},
		{
			name: "PublishIndexArticle",
			publish: func(ctx context.Context, c *Client) (string, error) {
				return c.PublishIndexArticle(ctx, IndexArticlePayload{ArticleID: "art-1", UserID: "user-1", FeedID: "feed-1"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, sc := validSpanContext(t)
			fake := &capturingMQHubClient{}
			c := &Client{client: fake, enabled: true}

			_, err := tc.publish(ctx, c)
			require.NoError(t, err)
			require.Len(t, fake.events, 1, "producer must publish exactly one event")

			md := fake.events[0].GetMetadata()
			require.NotNil(t, md, "event Metadata must not be nil")
			assert.Equal(t, sc.TraceID().String(), md["trace_id"],
				"producer must inject the span TraceID hex as metadata.trace_id (consumer pact requires it)")
			assert.Equal(t, sc.SpanID().String(), md["span_id"],
				"producer must inject the span SpanID hex as metadata.span_id")
		})
	}
}

// TestPublishDoesNotFabricateTraceIDWithoutSpan guards Rule 8/9: when no valid
// span rides on the context, the producer must NOT invent a trace_id. Absence of
// a span is a legitimate state (e.g. a code path outside any request/job span);
// injecting a fake value would make "no trace context" indistinguishable from a
// real one and would be a fabricated fact.
func TestPublishDoesNotFabricateTraceIDWithoutSpan(t *testing.T) {
	fake := &capturingMQHubClient{}
	c := &Client{client: fake, enabled: true}

	_, err := c.PublishArticleCreated(context.Background(), ArticleCreatedPayload{ArticleID: "art-1", UserID: "user-1", FeedID: "feed-1", Title: "t", URL: "https://example.com/a"})
	require.NoError(t, err)
	require.Len(t, fake.events, 1)

	md := fake.events[0].GetMetadata()
	_, hasTrace := md["trace_id"]
	assert.False(t, hasTrace, "producer must not fabricate a trace_id when no valid span is on the context")
	_, hasSpan := md["span_id"]
	assert.False(t, hasSpan, "producer must not fabricate a span_id when no valid span is on the context")
}
