package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
)

const operatorAuthHeader = "Authorization"

var errOperatorUnauthenticated = errors.New("unauthorized")

// OperatorAuthInterceptor requires "Authorization: Bearer <token>" on every
// RPC reaching cmd/backend's operator listener (:9102). It mirrors
// knowledge-sovereign's event-log interceptor: a constant-time comparison so
// a partial match cannot be inferred from response timing (lengths still
// differ, which is why tokens are fixed-size random secrets).
//
// Enabled is false only when the operator explicitly set OPERATOR_AUTH=disabled
// (config.LoadOperatorAuth) — an unset token is a startup failure there, never
// a silent pass-through here.
type OperatorAuthInterceptor struct {
	token   string
	enabled bool
}

// NewOperatorAuthInterceptor creates the operator-listener auth interceptor.
func NewOperatorAuthInterceptor(token string, enabled bool) *OperatorAuthInterceptor {
	return &OperatorAuthInterceptor{token: token, enabled: enabled}
}

// Interceptor returns a connect.Interceptor for use with Connect handlers.
func (o *OperatorAuthInterceptor) Interceptor() connect.Interceptor {
	return &operatorAuthInterceptor{parent: o}
}

type operatorAuthInterceptor struct {
	parent *OperatorAuthInterceptor
}

func (i *operatorAuthInterceptor) checkAuth(header http.Header) error {
	if !i.parent.enabled {
		return nil
	}
	const prefix = "Bearer "
	auth := header.Get(operatorAuthHeader)
	if !strings.HasPrefix(auth, prefix) {
		return connect.NewError(connect.CodeUnauthenticated, errOperatorUnauthenticated)
	}
	presented := strings.TrimPrefix(auth, prefix)
	if subtle.ConstantTimeCompare([]byte(presented), []byte(i.parent.token)) != 1 {
		return connect.NewError(connect.CodeUnauthenticated, errOperatorUnauthenticated)
	}
	return nil
}

// WrapUnary rejects unauthenticated unary RPCs before they reach the handler.
func (i *operatorAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.checkAuth(req.Header()); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient is a pass-through: this interceptor only guards the
// operator listener's inbound handler side, never an outbound client call.
func (i *operatorAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler rejects unauthenticated streaming RPCs before they
// reach the handler.
func (i *operatorAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if err := i.checkAuth(conn.RequestHeader()); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}
