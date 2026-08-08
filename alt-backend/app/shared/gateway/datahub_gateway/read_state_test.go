package datahub_gateway

import (
	"errors"
	"testing"

	"connectrpc.com/connect"

	"alt/domain"
)

// TestFeedAbsenceError_KeepsBothTheDomainErrorAndTheUpstreamCode pins that the
// domain translation does not erase the upstream Connect code. Callers that
// branch on domain.ErrFeedNotFound keep working, and callers that do not still
// reach the handler with a NotFound code instead of an anonymous error that
// degrades into a 500.
func TestFeedAbsenceError_KeepsBothTheDomainErrorAndTheUpstreamCode(t *testing.T) {
	t.Parallel()

	upstream := connect.NewError(connect.CodeNotFound, errors.New("feed not registered"))
	err := feedAbsenceError(upstream, "add favorite feed")

	if !errors.Is(err, domain.ErrFeedNotFound) {
		t.Fatalf("expected the domain absence error to stay reachable, got %v", err)
	}
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("expected the upstream CodeNotFound to stay reachable, got %v", got)
	}
}

// TestFeedAbsenceError_NonAbsenceKeepsUpstreamCode guards the other branch: a
// fault must not be relabelled as an absence, and its code must survive.
func TestFeedAbsenceError_NonAbsenceKeepsUpstreamCode(t *testing.T) {
	t.Parallel()

	upstream := connect.NewError(connect.CodeUnavailable, errors.New("data hub down"))
	err := feedAbsenceError(upstream, "mark feed read")

	if errors.Is(err, domain.ErrFeedNotFound) {
		t.Fatal("a fault must not be relabelled as a feed absence")
	}
	if got := connect.CodeOf(err); got != connect.CodeUnavailable {
		t.Fatalf("expected CodeUnavailable to stay reachable, got %v", got)
	}
}
