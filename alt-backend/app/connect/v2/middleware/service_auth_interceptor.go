// Package middleware provides Connect-RPC interceptors for authentication and other cross-cutting concerns.
package middleware

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
)

const (
	serviceTokenHeader = "X-Service-Token"
)

// ServiceAuthInterceptor provides shared-secret authentication for internal
// service-to-service Connect-RPC calls (no JWT / user context needed).
type ServiceAuthInterceptor struct {
	logger *slog.Logger
	secret string
}

// NewServiceAuthInterceptor creates a new interceptor that validates a shared secret.
func NewServiceAuthInterceptor(logger *slog.Logger, secret string) *ServiceAuthInterceptor {
	if logger == nil {
		logger = slog.Default()
	}
	if secret == "" {
		logger.Warn("SERVICE_SECRET not set, internal API will deny all requests")
	}
	return &ServiceAuthInterceptor{
		logger: logger,
		secret: secret,
	}
}

// Interceptor returns a connect.Interceptor for use with Connect handlers.
func (s *ServiceAuthInterceptor) Interceptor() connect.Interceptor {
	return &serviceInterceptor{auth: s}
}

type serviceInterceptor struct {
	auth *ServiceAuthInterceptor
}

func (i *serviceInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.auth.validate(req.Header().Get(serviceTokenHeader)); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

func (i *serviceInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}

func (i *serviceInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if err := i.auth.validate(conn.RequestHeader().Get(serviceTokenHeader)); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

func (s *ServiceAuthInterceptor) validate(token string) *connect.Error {
	if s.secret == "" {
		return connect.NewError(connect.CodeUnauthenticated, errMissingToken)
	}
	if token == "" {
		s.logger.Warn("internal API request missing service token")
		return connect.NewError(connect.CodeUnauthenticated, errMissingToken)
	}
	if token != s.secret {
		s.logger.Warn("internal API request with invalid service token")
		return connect.NewError(connect.CodeUnauthenticated, errInvalidToken)
	}
	return nil
}
