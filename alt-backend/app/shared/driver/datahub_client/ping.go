package datahub_client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// errUnavailable is opaque: Ping never wraps the transport error, so a
// deep-health handler can surface it without leaking the dial target.
var errUnavailable = errors.New("unavailable")

const maxDeepBody = 4 << 10

// Ping hits alt-data-hub's mTLS /health/deep — the data-path listener, not
// cheap liveness and not the ops port. pass/warn means the owned DB/pool
// answered; fail (or anything else) is unavailable.
func Ping(ctx context.Context, client *http.Client, baseURL string) error {
	if client == nil {
		return errUnavailable
	}
	target := strings.TrimRight(baseURL, "/") + "/health/deep"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return errUnavailable
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errUnavailable
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	var env struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxDeepBody)).Decode(&env); err != nil {
		return errUnavailable
	}
	switch env.Status {
	case "pass", "warn":
		return nil
	default:
		return errUnavailable
	}
}
