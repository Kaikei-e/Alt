package backend_api

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	datahubv1 "pre-processor/gen/proto/services/datahub/v1"
)

func TestClient_NilResponseMessageFailsFast(t *testing.T) {
	t.Parallel()

	nilMsgInterceptor := connect.UnaryInterceptorFunc(func(_ connect.UnaryFunc) connect.UnaryFunc {
		return func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
			var nilMsg *datahubv1.ListFeedURLsResponse
			return connect.NewResponse(nilMsg), nil
		}
	})

	c := newClientWithOptions("http://127.0.0.1:1", nil, connect.WithInterceptors(nilMsgInterceptor))
	_, err := c.DataHub().ListFeedURLs(context.Background(), connect.NewRequest(&datahubv1.ListFeedURLsRequest{}))
	if err == nil {
		t.Fatal("ListFeedURLs() error = nil, want nil response message error")
	}
	if !strings.Contains(err.Error(), "nil response message") {
		t.Fatalf("ListFeedURLs() error = %v, want substring %q", err, "nil response message")
	}
}
