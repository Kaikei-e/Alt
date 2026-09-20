package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
)

type eventAuthInterceptor struct {
	token   string
	enabled bool
}

// NewEventAuthInterceptor creates a Connect-RPC interceptor requiring
// Authorization: Bearer <token> on every incoming RPC.
// Pass-through happens only when enabled is false (EVENT_AUTH=disabled).
// When enabled is true, empty or mismatched tokens are rejected with
// connect.CodeUnauthenticated, returning HTTP 401 over Connect/HTTP protocol.
func NewEventAuthInterceptor(token string, enabled bool) connect.Interceptor {
	return &eventAuthInterceptor{
		token:   token,
		enabled: enabled,
	}
}

func (i *eventAuthInterceptor) checkAuth(header http.Header) error {
	if !i.enabled {
		return nil
	}
	if i.token == "" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	const prefix = "Bearer "
	auth := header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) ||
		subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, prefix)), []byte(i.token)) != 1 {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	return nil
}

func (i *eventAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.checkAuth(req.Header()); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

func (i *eventAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *eventAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if err := i.checkAuth(conn.RequestHeader()); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}
