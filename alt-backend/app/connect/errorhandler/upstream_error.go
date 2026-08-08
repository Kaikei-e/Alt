package errorhandler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
)

// callerClassUpstreamCodes are the Connect codes that the Connect protocol maps
// to a 4xx HTTP status: the request was malformed or unauthorised, the upstream
// itself was healthy. Relaying one of these as CodeInternal claims an outage
// that never happened and spends the 5xx dependency-failure budget that
// downstream circuit breakers count.
//
// Every other code is deliberately absent — internal, unknown, unavailable,
// data_loss, deadline_exceeded, unimplemented and canceled all stay 5xx and
// stay loud. Making an unclassifiable failure quietly non-5xx would swap one
// dishonesty for another (CLAUDE.md rule 8).
var callerClassUpstreamCodes = map[connect.Code]struct{}{
	connect.CodeInvalidArgument:    {},
	connect.CodeNotFound:           {},
	connect.CodeAlreadyExists:      {},
	connect.CodePermissionDenied:   {},
	connect.CodeResourceExhausted:  {},
	connect.CodeFailedPrecondition: {},
	connect.CodeAborted:            {},
	connect.CodeOutOfRange:         {},
	connect.CodeUnauthenticated:    {},
}

// HandleUpstreamError converts an error that may carry a Connect code from an
// upstream service (knowledge-sovereign, alt-data-hub, rag-orchestrator, ...)
// into the Connect error this service returns to its own caller.
//
// A caller-class upstream code keeps its class and logs at WARN carrying the
// operation, the upstream code and the upstream message verbatim: alt-backend
// sending a malformed request is still a bug, and the upstream message is the
// only thing that says which field was wrong. Anything else — including an
// error with no Connect code at all — falls through to HandleInternalError so
// genuine faults stay 5xx, sanitised, and correlated by error_id.
func HandleUpstreamError(ctx context.Context, logger *slog.Logger, err error, operation string) *connect.Error {
	upstream, ok := callerClassUpstream(err)
	if !ok {
		return HandleInternalError(ctx, logger, err, operation)
	}

	logger.WarnContext(ctx,
		"Connect-RPC Upstream Client Error",
		"operation", operation,
		"upstream_code", upstream.Code().String(),
		"upstream_message", upstream.Message(),
		"cause", err.Error(),
	)

	message := upstream.Message()
	if message == "" {
		message = upstream.Code().String()
	}
	return connect.NewError(upstream.Code(), fmt.Errorf("%s: %s", operation, message))
}

// callerClassUpstream returns the upstream *connect.Error when the chain
// carries one whose code is caller-class. Classification is on the typed error
// only — never on error text, which reformats on every wrap.
func callerClassUpstream(err error) (*connect.Error, bool) {
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return nil, false
	}
	if _, ok := callerClassUpstreamCodes[connectErr.Code()]; !ok {
		return nil, false
	}
	return connectErr, true
}
