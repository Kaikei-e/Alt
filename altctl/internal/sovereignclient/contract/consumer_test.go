//go:build contract

// Package contract contains Consumer-Driven Contract tests for
// altctl → knowledge-sovereign's admin REST API (port :9501).
//
// The admin surface is operator-critical (disaster recovery: snapshot
// create / list / latest, retention eligible / run --dry-run, storage
// stats). A silent wire-format drift would break `altctl home snapshot`
// and `altctl home retention` in the middle of an incident response.
// These interactions pin:
//
//   - Snapshot response fields are PascalCase (encoding/json default on
//     sovereign's untagged struct). See ADR-000765 §3.
//   - EventSeqBoundary is a positive integer — append-first invariant.
//   - SnapshotID is a UUIDv4.
//   - Checksums are `sha256:<64 hex chars>` literals.
//   - Retention dry-run echoes `dry_run: true` and omits `error`.
package contract

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alt-project/altctl/internal/sovereignclient"
)

// Pact files go under altctl/pacts/ (picked up by scripts/pact-check.sh
// publish scan).
const pactDir = "../../../pacts"

func newPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mp, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "altctl",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mp
}

func clientFor(config consumer.MockServerConfig) *sovereignclient.SovereignClient {
	return sovereignclient.NewClient(fmt.Sprintf("http://%s:%d", config.Host, config.Port))
}

// snapshotPayload captures the PascalCase fields altctl decodes from
// sovereign's snapshot endpoints.
type snapshotPayload struct {
	SnapshotID       string `json:"SnapshotID"`
	EventSeqBoundary int64  `json:"EventSeqBoundary"`
	ItemsChecksum    string `json:"ItemsChecksum"`
	VersionsChecksum string `json:"VersionsChecksum"`
	DedupesChecksum  string `json:"DedupesChecksum"`
	CreatedAt        string `json:"CreatedAt"`
}

func TestSnapshotCreate(t *testing.T) {
	mp := newPact(t)

	err := mp.
		AddInteraction().
		Given("an admin operator has snapshot authority").
		UponReceiving("a snapshot create request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/admin/snapshots/create"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"SnapshotID":       matchers.Regex("11111111-2222-3333-4444-555555555555", "^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$"),
				"EventSeqBoundary": matchers.Like(1),
				"ItemsChecksum":    matchers.Regex("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "^sha256:[0-9a-f]{64}$"),
				"VersionsChecksum": matchers.Regex("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "^sha256:[0-9a-f]{64}$"),
				"DedupesChecksum":  matchers.Regex("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "^sha256:[0-9a-f]{64}$"),
				"CreatedAt":        matchers.Like("2026-04-23T00:00:00Z"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp snapshotPayload
			if err := c.Post(context.Background(), "/admin/snapshots/create", map[string]any{}, &resp); err != nil {
				return err
			}
			assert.NotEmpty(t, resp.SnapshotID)
			assert.Greater(t, resp.EventSeqBoundary, int64(0), "EventSeqBoundary must be > 0 (append-first invariant)")
			assert.Contains(t, resp.ItemsChecksum, "sha256:")
			return nil
		})
	require.NoError(t, err)
}

func TestSnapshotLatest(t *testing.T) {
	mp := newPact(t)

	err := mp.
		AddInteraction().
		Given("at least one snapshot exists").
		UponReceiving("a latest snapshot query").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/admin/snapshots/latest"),
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"SnapshotID":       matchers.Regex("11111111-2222-3333-4444-555555555555", "^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$"),
				"EventSeqBoundary": matchers.Like(1),
				"ItemsChecksum":    matchers.Regex("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "^sha256:[0-9a-f]{64}$"),
				"VersionsChecksum": matchers.Regex("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "^sha256:[0-9a-f]{64}$"),
				"DedupesChecksum":  matchers.Regex("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "^sha256:[0-9a-f]{64}$"),
				"CreatedAt":        matchers.Like("2026-04-23T00:00:00Z"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp snapshotPayload
			return c.Get(context.Background(), "/admin/snapshots/latest", &resp)
		})
	require.NoError(t, err)
}

func TestSnapshotList(t *testing.T) {
	mp := newPact(t)

	err := mp.
		AddInteraction().
		Given("at least one snapshot exists").
		UponReceiving("a snapshot list query").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/admin/snapshots/list"),
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.EachLike(
				matchers.MapMatcher{
					"SnapshotID":       matchers.Regex("11111111-2222-3333-4444-555555555555", "^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$"),
					"EventSeqBoundary": matchers.Like(1),
					"ItemsChecksum":    matchers.Regex("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "^sha256:[0-9a-f]{64}$"),
					"CreatedAt":        matchers.Like("2026-04-23T00:00:00Z"),
				},
				1,
			),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp []snapshotPayload
			return c.Get(context.Background(), "/admin/snapshots/list", &resp)
		})
	require.NoError(t, err)
}

// retentionEligibleRow matches the PascalCase row shape altctl decodes.
type retentionEligibleRow struct {
	Table         string `json:"Table"`
	PartitionName string `json:"PartitionName"`
	EventSeqMax   int64  `json:"EventSeqMax"`
}

func TestRetentionEligible(t *testing.T) {
	mp := newPact(t)

	err := mp.
		AddInteraction().
		Given("retention policies are configured").
		UponReceiving("a retention eligible query").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/admin/retention/eligible"),
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.EachLike(
				matchers.MapMatcher{
					"Table":         matchers.Like("knowledge_events"),
					"PartitionName": matchers.Like("knowledge_events_y2025m01"),
					"EventSeqMax":   matchers.Like(1),
				},
				1,
			),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp []retentionEligibleRow
			return c.Get(context.Background(), "/admin/retention/eligible", &resp)
		})
	require.NoError(t, err)
}

func TestRetentionRunDry(t *testing.T) {
	mp := newPact(t)

	err := mp.
		AddInteraction().
		Given("retention policies are configured").
		UponReceiving("a retention dry-run request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/admin/retention/run"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"dry_run": matchers.Like(true),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"dry_run": matchers.Like(true),
				"actions": matchers.EachLike(
					matchers.MapMatcher{
						"table":          matchers.Like("knowledge_events"),
						"partition_name": matchers.Like("knowledge_events_y2025m01"),
						"action":         matchers.Like("would_archive"),
					},
					1,
				),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp struct {
				DryRun  bool             `json:"dry_run"`
				Actions []map[string]any `json:"actions"`
			}
			return c.Post(context.Background(), "/admin/retention/run", map[string]any{"dry_run": true}, &resp)
		})
	require.NoError(t, err)
}
