package articles

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	"connectrpc.com/connect"

	"alt/connect/v2/middleware"
	"alt/domain"
)

const (
	// FailureScopeHeader names how far a failure reaches. Connect error
	// metadata is merged into the unary response headers, so it survives the
	// BFF's transparent proxy and reaches connect-es as ConnectError.metadata.
	//
	// It exists because CodeUnavailable is issued by two parties that mean
	// opposite things by it: alt-backend for a single publisher that did not
	// answer, and the BFF for a breaker that is open against every host. A
	// client that cannot tell them apart must guess, and guessing "global"
	// let one dead link black the whole reader out for a full cooldown.
	FailureScopeHeader = "X-Alt-Failure-Scope"

	// FailureScopeHost means the failure belongs to one third-party host.
	// Every other host is still reachable and alt-backend is healthy, so this
	// must not charge a shared failure budget or pause unrelated work.
	//
	// Only stamp it on errors positively attributed to a publisher. Our own
	// politeness gate is not a publisher's health, and an unclassified fault
	// is still ours — excusing either from the breaker would hide a real
	// outage behind "the site is slow".
	FailureScopeHost = "host"
)

// withFailureScope stamps the blast radius onto a Connect error.
func withFailureScope(err *connect.Error, scope string) *connect.Error {
	err.Meta().Set(FailureScopeHeader, scope)
	return err
}

// requireUser authenticates the request context and returns the domain user context.
func requireUser(ctx context.Context) (*domain.UserContext, error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	return user, nil
}

// mapExternalFetchError maps domain errors from external article fetches into Connect errors.
func mapExternalFetchError(err error) *connect.Error {
	// Checked before ComplianceError: a politeness gate that has not
	// elapsed is transient and must reach the client as a retryable 429,
	// not as the permanent 403 that tells it to stop asking.
	var rateErr *domain.RateLimitedError
	if errors.As(err, &rateErr) {
		connectErr := connect.NewError(connect.CodeResourceExhausted,
			fmt.Errorf("%s", rateErr.Message))
		if rateErr.RetryAfter > 0 {
			connectErr.Meta().Set("Retry-After",
				strconv.Itoa(int(math.Ceil(rateErr.RetryAfter.Seconds()))))
		}
		return connectErr
	}

	var complianceErr *domain.ComplianceError
	if errors.As(err, &complianceErr) {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("%s", complianceErr.Message))
	}

	// The publisher answered, just not with what we wanted. Whatever the
	// status, the verdict is about that one site — hence FailureScopeHost
	// on every branch, including the CodeUnavailable default that is
	// otherwise indistinguishable from the BFF's own breaker rejection.
	var httpErr *domain.ExternalHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case 404, 410:
			return withFailureScope(connect.NewError(connect.CodeNotFound,
				fmt.Errorf("article not found")), FailureScopeHost)
		case 403, 401:
			return withFailureScope(connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("access denied by external site")), FailureScopeHost)
		case 429:
			return withFailureScope(connect.NewError(connect.CodeResourceExhausted,
				fmt.Errorf("rate limited by external site")), FailureScopeHost)
		default:
			return withFailureScope(connect.NewError(connect.CodeUnavailable,
				fmt.Errorf("external site returned %d", httpErr.StatusCode)), FailureScopeHost)
		}
	}

	// The publisher never answered — a slow site, a dead host, or a wait
	// that ran out. alt-backend is healthy, so this is neither an internal
	// fault to the client nor an ERROR line for whoever reads the log.
	var upstreamErr *domain.UpstreamFetchError
	if errors.As(err, &upstreamErr) {
		return withFailureScope(connect.NewError(connect.CodeUnavailable,
			fmt.Errorf("the source site did not respond; please try again later")),
			FailureScopeHost)
	}

	return nil
}

// isUpstreamFetchWinner returns the UpstreamFetchError only if it is the winning error branch
// (i.e. no earlier RateLimited, Compliance, or ExternalHTTP error matched).
func isUpstreamFetchWinner(err error) *domain.UpstreamFetchError {
	var rateErr *domain.RateLimitedError
	var complianceErr *domain.ComplianceError
	var httpErr *domain.ExternalHTTPError
	if errors.As(err, &rateErr) || errors.As(err, &complianceErr) || errors.As(err, &httpErr) {
		return nil
	}
	var upstreamErr *domain.UpstreamFetchError
	if errors.As(err, &upstreamErr) {
		return upstreamErr
	}
	return nil
}
