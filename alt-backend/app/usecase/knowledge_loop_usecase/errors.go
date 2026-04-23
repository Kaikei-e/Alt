package knowledge_loop_usecase

import (
	"context"
	"errors"
	"net"
)

// ErrUpstreamUnavailable marks a transient failure reaching a dependency
// (sovereign gRPC, pgbouncer, etc.). Handlers map it to Connect CodeUnavailable
// so the SvelteKit BFF can surface it as HTTP 502 with retry guidance —
// distinct from CodeInternal which indicates an invariant violation.
var ErrUpstreamUnavailable = errors.New("upstream unavailable")

// ClassifyDriverError wraps a driver/port-level error with the appropriate
// usecase sentinel so the handler can route it to the correct Connect-RPC
// code. It preserves the original chain via %w so errors.Is / errors.As still
// work on the raw cause.
//
// Classification rules (first match wins):
//
//   - context.DeadlineExceeded / Canceled → returned as-is; handler dispatches
//     on context errors directly. Do not wrap: deadline sentinels should not
//     be hidden behind an "unavailable" category.
//   - any net.Error (dial errors, timeouts, resets) → ErrUpstreamUnavailable.
//   - default → returned as-is; handler will log + map to CodeInternal.
func ClassifyDriverError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return errorsJoin(ErrUpstreamUnavailable, err)
	}
	return err
}

// errorsJoin wraps cause with sentinel using errors.Join semantics so that
// errors.Is(result, sentinel) AND errors.Is(result, cause) both hold.
func errorsJoin(sentinel, cause error) error {
	return errors.Join(sentinel, cause)
}
