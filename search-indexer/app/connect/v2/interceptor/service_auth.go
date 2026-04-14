// Package interceptor provides Connect-RPC interceptors for search-indexer.
package interceptor

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"search-indexer/logger"
)

const serviceTokenHeader = "X-Service-Token"

// ServiceAuthInterceptor enforces X-Service-Token on every Connect-RPC call.
// It mirrors the REST middleware contract so internal callers see a uniform
// authentication model across transports.
type ServiceAuthInterceptor struct {
	serviceSecret string
}

// NewServiceAuthInterceptor constructs an interceptor bound to the shared
// secret. An empty secret causes every request to be rejected (fail-closed).
func NewServiceAuthInterceptor(serviceSecret string) *ServiceAuthInterceptor {
	return &ServiceAuthInterceptor{serviceSecret: serviceSecret}
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

// WrapStreamingClient implements connect.Interceptor. search-indexer does not
// expose streaming endpoints today; adding the token at construction time is
// still correct for any future streaming client.
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
		logger.Logger.ErrorContext(ctx, "connect service auth misconfigured: SERVICE_TOKEN is empty", "procedure", procedure)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("service authentication not configured"))
	}

	token = strings.TrimSpace(token)
	if token == "" {
		logger.Logger.WarnContext(ctx, "connect service auth failed: missing token", "procedure", procedure)
		return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing X-Service-Token"))
	}

	expected := []byte(i.serviceSecret)
	provided := []byte(token)
	valid := len(expected) == len(provided) &&
		subtle.ConstantTimeCompare(expected, provided) == 1
	if !valid {
		logger.Logger.WarnContext(ctx, "connect service auth failed: invalid token", "procedure", procedure)
		return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid X-Service-Token"))
	}

	return nil
}

// Assert compile-time interface compliance.
var _ connect.Interceptor = (*ServiceAuthInterceptor)(nil)
