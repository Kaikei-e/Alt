# CLAUDE.md - Pre-processor Sidecar (Scheduler)

## About This Service

> Detailed scheduler/token rotation status is captured in `docs/pre-processor-sidecar.md`.

The pre-processor-sidecar is a Go scheduler responsible for orchestrating RSS ingestion via the Inoreader API. It runs as a Kubernetes CronJob by default and can switch to a long-running Deployment in "schedule-mode" for debugging.

- **Language**: Go 1.24+
- **Role**: Scheduler & OAuth2 Token Manager
- **Integrations**: Inoreader OAuth2, auth-token-manager, Kubernetes Secrets
- **Runtime**: Kubernetes CronJob (production), Deployment (debugging)

## Core Responsibilities

-   **Scheduling**: Trigger periodic article fetches and subscription synchronization.
-   **Token Management**: Maintain and rotate OAuth2 tokens, persisting them to Kubernetes Secrets.
-   **Resilience**: Enforce external rate limits and implement circuit breaking and single-flight concurrency control.
-   **Administration**: Provide minimal admin endpoints for token health and manual job triggers.

## TDD and Testing Strategy

### TDD Guidelines
-   **Red-Green-Refactor**: This cycle is mandatory for all logic in the `handler` and `service` packages.
-   **Mock Dependencies**: Mock all external drivers, including the OAuth2 provider, API clients, and token repositories.
-   **No Network Calls**: Unit tests must not make any real network calls.

### Testing Time-Sensitive Logic
Testing logic like token expiry and rotation requires control over time. Use an interface for time-related functions.

```go
// time.go
package main

import "time"

// Clock is an interface for time-related functions.
type Clock interface {
    Now() time.Time
}

// RealClock implements Clock using the real time.	ype RealClock struct{}

func (c RealClock) Now() time.Time {
    return time.Now()
}

// MockClock is a mock implementation of Clock for testing.
type MockClock struct {
    currentTime time.Time
}

func (c *MockClock) Now() time.Time {
    return c.currentTime
}
```

Inject the `Clock` interface into your services to control time in your tests.

## Concurrency Control with Single-Flight

To prevent redundant operations, such as multiple concurrent token refresh calls, we use the `single-flight` pattern. This ensures that for a given key, a function is only executed once, and all concurrent callers receive the same result.

```go
import "golang.org/x/sync/singleflight"

var g singleflight.Group

func (s *TokenService) RefreshToken(ctx context.Context) (*oauth2.Token, error) {
    // The key ensures that only one refresh operation runs at a time.
    key := "refreshToken"

    v, err, _ := g.Do(key, func() (interface{}, error) {
        // Actual token refresh logic here...
        return s.performTokenRefresh(ctx)
    })

    if err != nil {
        return nil, err
    }

    return v.(*oauth2.Token), nil
}
```

## Reliability and Kubernetes Configuration

### CronJob Best Practices
-   **`concurrencyPolicy: Forbid`**: This is critical to prevent multiple instances of the job from running simultaneously if a previous job is still running.
-   **`startingDeadlineSeconds`**: Set a deadline to prevent jobs from running at unexpected times after being delayed.
-   **Resource Limits**: Always define CPU and memory requests and limits.

### OAuth2 Token Rotation
-   **Proactive Refresh**: Refresh tokens before they expire to avoid API call failures.
-   **Refresh Token Rotation**: For enhanced security, use a "one-time-use" policy for refresh tokens if the provider supports it.

## Security

-   **Never Log Secrets**: Never log tokens or other sensitive credentials. Use sanitized logging.
-   **Kubernetes Secrets**: Prefer using Kubernetes Secrets for token storage in production.
-   **Principle of Least Privilege**: The sidecar's service account should only have permissions to manage its own secrets.

## References

-   [Testing Kubernetes CronJobs](https://codingexplorations.com/blog/mastering-go-in-the-cloud-cronjobs-oauth2-and-concurrency)
-   [Go `singleflight` Package](https://pkg.go.dev/golang.org/x/sync/singleflight)
-   [OAuth2 for Go](https://pkg.go.dev/golang.org/x/oauth2)
