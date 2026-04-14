package interceptor

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"golang.org/x/time/rate"

	"search-indexer/logger"
)

// RateLimitInterceptor enforces a global token bucket on every Connect-RPC
// call. Excess calls receive CodeResourceExhausted, which Connect translates
// to HTTP 429 / gRPC status 8.
type RateLimitInterceptor struct {
	limiter *rate.Limiter
}

func NewRateLimitInterceptor(r rate.Limit, burst int) *RateLimitInterceptor {
	return &RateLimitInterceptor{limiter: rate.NewLimiter(r, burst)}
}

func (i *RateLimitInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !i.limiter.Allow() {
			logger.Logger.WarnContext(ctx, "connect rate limit exceeded",
				"procedure", req.Spec().Procedure)
			return nil, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("rate limit exceeded"))
		}
		return next(ctx, req)
	}
}

func (i *RateLimitInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *RateLimitInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if !i.limiter.Allow() {
			return connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("rate limit exceeded"))
		}
		return next(ctx, conn)
	}
}

var _ connect.Interceptor = (*RateLimitInterceptor)(nil)
