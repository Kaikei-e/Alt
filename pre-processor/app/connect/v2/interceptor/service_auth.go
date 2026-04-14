// Package interceptor provides Connect-RPC interceptors for pre-processor.
package interceptor

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
)

const serviceTokenHeader = "X-Service-Token"

// ServiceAuthInterceptor enforces X-Service-Token on every Connect-RPC call.
// It mirrors the REST ServiceAuthMiddleware so internal callers see a uniform
// authentication model across transports.
type ServiceAuthInterceptor struct {
	logger        *slog.Logger
	serviceSecret string
}

// NewServiceAuthInterceptor constructs an interceptor bound to the shared
// secret. An empty secret causes every request to be rejected (fail-closed).
func NewServiceAuthInterceptor(logger *slog.Logger, serviceSecret string) *ServiceAuthInterceptor {
	if logger == nil {
		logger = slog.Default()
	}
	return &ServiceAuthInterceptor{logger: logger, serviceSecret: serviceSecret}
}

// WrapUnary implements connect.Interceptor.
func (i *ServiceAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.verify(ctx, req.Header().Get(serviceTokenHeader), req.Spec().Procedure); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient implements connect.Interceptor.
func (i *ServiceAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements connect.Interceptor. Rejects streams that
// lack a valid X-Service-Token before the handler starts producing data.
func (i *ServiceAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if err := i.verify(ctx, conn.RequestHeader().Get(serviceTokenHeader), conn.Spec().Procedure); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

func (i *ServiceAuthInterceptor) verify(ctx context.Context, token, procedure string) error {
	if i.serviceSecret == "" {
		i.logger.ErrorContext(ctx, "connect service auth misconfigured: SERVICE_SECRET is empty", "procedure", procedure)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("service authentication not configured"))
	}

	token = strings.TrimSpace(token)
	if token == "" {
		i.logger.WarnContext(ctx, "connect service auth failed: missing token", "procedure", procedure)
		return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing X-Service-Token"))
	}

	expected := []byte(i.serviceSecret)
	provided := []byte(token)
	valid := len(expected) == len(provided) &&
		subtle.ConstantTimeCompare(expected, provided) == 1
	if !valid {
		i.logger.WarnContext(ctx, "connect service auth failed: invalid token", "procedure", procedure)
		return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid X-Service-Token"))
	}

	return nil
}

var _ connect.Interceptor = (*ServiceAuthInterceptor)(nil)
