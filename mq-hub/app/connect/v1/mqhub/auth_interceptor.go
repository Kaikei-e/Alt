package mqhub

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"connectrpc.com/connect"

	mqhubv1connect "mq-hub/gen/proto/services/mqhub/v1/mqhubv1connect"
)

var (
	ErrMissingAuthHeader = errors.New("missing or invalid authorization header")
	ErrInvalidAuthToken  = errors.New("invalid authentication token")
)

// AuthInterceptor enforces Bearer token authentication on all business RPCs.
// HealthCheck procedure is explicitly exempt.
type AuthInterceptor struct {
	authToken string
}

// NewAuthInterceptor creates a new AuthInterceptor.
func NewAuthInterceptor(authToken string) *AuthInterceptor {
	return &AuthInterceptor{
		authToken: authToken,
	}
}

// WrapUnary validates the Bearer token for unary RPCs.
func (a *AuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		// Exact exemption: HealthCheck procedure
		if req.Spec().Procedure == mqhubv1connect.MQHubServiceHealthCheckProcedure {
			return next(ctx, req)
		}

		authHeader := req.Header().Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return nil, connect.NewError(connect.CodeUnauthenticated, ErrMissingAuthHeader)
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(a.authToken)) != 1 {
			return nil, connect.NewError(connect.CodeUnauthenticated, ErrInvalidAuthToken)
		}

		return next(ctx, req)
	}
}

// WrapStreamingClient passes through streaming client connections.
func (a *AuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}

// WrapStreamingHandler validates the Bearer token for streaming RPCs.
func (a *AuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if conn.Spec().Procedure == mqhubv1connect.MQHubServiceHealthCheckProcedure {
			return next(ctx, conn)
		}

		authHeader := conn.RequestHeader().Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return connect.NewError(connect.CodeUnauthenticated, ErrMissingAuthHeader)
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(a.authToken)) != 1 {
			return connect.NewError(connect.CodeUnauthenticated, ErrInvalidAuthToken)
		}

		return next(ctx, conn)
	}
}
