package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
)

// ContextCancelInterceptor catches context cancellation/timeout errors at the RPC boundary.
// It maps them to appropriate Connect-RPC codes and logs at INFO level (not ERROR),
// since client disconnects are expected operational events.
type ContextCancelInterceptor struct {
	logger *slog.Logger
}

// NewContextCancelInterceptor creates a new context cancellation interceptor.
func NewContextCancelInterceptor(logger *slog.Logger) *ContextCancelInterceptor {
	if logger == nil {
		logger = slog.Default()
	}
	return &ContextCancelInterceptor{logger: logger}
}

// Interceptor returns a connect.Interceptor for use with Connect handlers.
func (c *ContextCancelInterceptor) Interceptor() connect.Interceptor {
	return &contextCancelInterceptor{parent: c}
}

type contextCancelInterceptor struct {
	parent *ContextCancelInterceptor
}

// WrapUnary handles context cancellation for unary RPCs.
// Returns CodeCanceled or CodeDeadlineExceeded with INFO-level logging.
func (i *contextCancelInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)
		if err != nil && ctx.Err() != nil {
			code := connect.CodeCanceled
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				code = connect.CodeDeadlineExceeded
			}
			procedure := ""
			if req != nil {
				procedure = req.Spec().Procedure
			}
			i.parent.logger.InfoContext(ctx, "RPC cancelled",
				"procedure", procedure,
				"reason", ctx.Err())
			return nil, connect.NewError(code, fmt.Errorf("request cancelled"))
		}
		return resp, err
	}
}

// WrapStreamingClient is a pass-through for client streaming (no server-side logic needed).
func (i *contextCancelInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}

// WrapStreamingHandler handles context cancellation for server streaming RPCs.
// Returns nil when the client has disconnected (nothing to send to).
func (i *contextCancelInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		err := next(ctx, conn)
		if err != nil && ctx.Err() != nil {
			procedure := ""
			if conn != nil {
				procedure = conn.Spec().Procedure
			}
			i.parent.logger.InfoContext(ctx, "streaming RPC cancelled",
				"procedure", procedure,
				"reason", ctx.Err())
			return nil
		}
		return err
	}
}
