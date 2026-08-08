package sovereign_client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

type rejectingUserEventHandler struct {
	sovereignv1connect.UnimplementedKnowledgeSovereignServiceHandler
	code    connect.Code
	message string
}

func (h *rejectingUserEventHandler) AppendKnowledgeUserEvent(
	context.Context,
	*connect.Request[sovereignv1.AppendKnowledgeUserEventRequest],
) (*connect.Response[sovereignv1.AppendKnowledgeUserEventResponse], error) {
	return nil, connect.NewError(h.code, errors.New(h.message))
}

// TestAppendKnowledgeUserEvent_PreservesUpstreamCodeThroughWrap pins the driver
// end of the chain: the fmt.Errorf wrap must keep the *connect.Error reachable
// by errors.As so the handler boundary can map the code faithfully. A wrap that
// drops the chain (e.g. %v) would silently re-flatten every upstream 4xx into a
// 500 without failing any other test.
func TestAppendKnowledgeUserEvent_PreservesUpstreamCodeThroughWrap(t *testing.T) {
	upstream := &rejectingUserEventHandler{
		code:    connect.CodeInvalidArgument,
		message: "dedupe_key is required",
	}
	mux := http.NewServeMux()
	path, h := sovereignv1connect.NewKnowledgeSovereignServiceHandler(upstream)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, true)

	err := client.AppendKnowledgeUserEvent(context.Background(), domain.KnowledgeUserEvent{
		UserEventID: uuid.New(),
		UserID:      uuid.New(),
		TenantID:    uuid.New(),
		EventType:   "open",
		ItemKey:     "article:1",
		DedupeKey:   "",
	})

	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err),
		"the driver wrap must keep the upstream Connect code reachable")

	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	require.Equal(t, "dedupe_key is required", connectErr.Message(),
		"the upstream message must survive the wrap so the caller bug stays findable")
}
