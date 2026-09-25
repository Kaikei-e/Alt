package articles

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

func TestMapExternalFetchError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantNil        bool
		wantCode       connect.Code
		wantScope      string
		wantRetryAfter string
	}{
		{
			name: "rate limited with retry after",
			err: &domain.RateLimitedError{
				Message:    "rate limited",
				RetryAfter: 12 * time.Second,
			},
			wantNil:        false,
			wantCode:       connect.CodeResourceExhausted,
			wantScope:      "",
			wantRetryAfter: "12",
		},
		{
			name: "rate limited without retry after",
			err: &domain.RateLimitedError{
				Message:    "rate limited zero",
				RetryAfter: 0,
			},
			wantNil:        false,
			wantCode:       connect.CodeResourceExhausted,
			wantScope:      "",
			wantRetryAfter: "",
		},
		{
			name: "compliance error",
			err: &domain.ComplianceError{
				Message: "forbidden by robots.txt",
			},
			wantNil:        false,
			wantCode:       connect.CodePermissionDenied,
			wantScope:      "",
			wantRetryAfter: "",
		},
		{
			name: "external http 404",
			err: &domain.ExternalHTTPError{
				StatusCode: 404,
				URL:        "https://example.com/notfound",
			},
			wantNil:   false,
			wantCode:  connect.CodeNotFound,
			wantScope: FailureScopeHost,
		},
		{
			name: "external http 410",
			err: &domain.ExternalHTTPError{
				StatusCode: 410,
				URL:        "https://example.com/gone",
			},
			wantNil:   false,
			wantCode:  connect.CodeNotFound,
			wantScope: FailureScopeHost,
		},
		{
			name: "external http 403",
			err: &domain.ExternalHTTPError{
				StatusCode: 403,
				URL:        "https://example.com/denied",
			},
			wantNil:   false,
			wantCode:  connect.CodePermissionDenied,
			wantScope: FailureScopeHost,
		},
		{
			name: "external http 401",
			err: &domain.ExternalHTTPError{
				StatusCode: 401,
				URL:        "https://example.com/unauth",
			},
			wantNil:   false,
			wantCode:  connect.CodePermissionDenied,
			wantScope: FailureScopeHost,
		},
		{
			name: "external http 429",
			err: &domain.ExternalHTTPError{
				StatusCode: 429,
				URL:        "https://example.com/too-many",
			},
			wantNil:   false,
			wantCode:  connect.CodeResourceExhausted,
			wantScope: FailureScopeHost,
		},
		{
			name: "external http 500 default",
			err: &domain.ExternalHTTPError{
				StatusCode: 500,
				URL:        "https://example.com/error",
			},
			wantNil:   false,
			wantCode:  connect.CodeUnavailable,
			wantScope: FailureScopeHost,
		},
		{
			name: "upstream fetch error",
			err: &domain.UpstreamFetchError{
				URL:   "https://example.com/timeout",
				Cause: context.DeadlineExceeded,
			},
			wantNil:   false,
			wantCode:  connect.CodeUnavailable,
			wantScope: FailureScopeHost,
		},
		{
			name:    "unclassified error returns nil",
			err:     errors.New("generic internal error"),
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapExternalFetchError(tt.err)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.wantCode, got.Code())
			assert.Equal(t, tt.wantScope, got.Meta().Get(FailureScopeHeader))
			if tt.wantRetryAfter != "" {
				assert.Equal(t, tt.wantRetryAfter, got.Meta().Get("Retry-After"))
			}
		})
	}
}

func TestWithFailureScope(t *testing.T) {
	connectErr := connect.NewError(connect.CodeUnavailable, errors.New("upstream failed"))
	stamped := withFailureScope(connectErr, FailureScopeHost)
	assert.Equal(t, FailureScopeHost, stamped.Meta().Get(FailureScopeHeader))
}

func TestRequireUser(t *testing.T) {
	t.Run("authenticated context", func(t *testing.T) {
		userID := uuid.New()
		ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
			UserID:    userID,
			Email:     "user@example.com",
			Role:      domain.UserRoleUser,
			ExpiresAt: time.Now().Add(time.Hour),
		})
		user, err := requireUser(ctx)
		require.NoError(t, err)
		require.NotNil(t, user)
		assert.Equal(t, userID, user.UserID)
	})

	t.Run("unauthenticated context", func(t *testing.T) {
		ctx := context.Background()
		user, err := requireUser(ctx)
		assert.Nil(t, user)
		require.Error(t, err)

		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
	})
}

func TestIsUpstreamFetchWinner(t *testing.T) {
	upstream := &domain.UpstreamFetchError{URL: "https://example.com", Cause: context.DeadlineExceeded}
	httpErr := &domain.ExternalHTTPError{StatusCode: 404, URL: "https://example.com"}
	rateErr := &domain.RateLimitedError{Message: "busy"}
	complianceErr := &domain.ComplianceError{Message: "robots.txt"}

	// Upstream alone wins
	assert.Equal(t, upstream, isUpstreamFetchWinner(upstream))

	// When earlier branch is wrapped, earlier branch wins -> returns nil
	wrappedWithHTTP := errors.Join(upstream, httpErr)
	assert.Nil(t, isUpstreamFetchWinner(wrappedWithHTTP))

	wrappedWithRate := errors.Join(upstream, rateErr)
	assert.Nil(t, isUpstreamFetchWinner(wrappedWithRate))

	wrappedWithCompliance := errors.Join(upstream, complianceErr)
	assert.Nil(t, isUpstreamFetchWinner(wrappedWithCompliance))

	// Generic error returns nil
	assert.Nil(t, isUpstreamFetchWinner(errors.New("generic error")))
}
