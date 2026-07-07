//go:build contract

// Package contract contains Consumer-Driven Contract tests for
// altctl → knowledge-sovereign's admin REST API (port :9501).
//
// The admin surface is operator-critical (disaster recovery: snapshot
// create / list / latest, retention status / eligible / run --dry-run,
// storage stats). A silent wire-format drift would break `altctl home
// snapshot`, `altctl home retention`, and `altctl home storage` in the
// middle of an incident response. These interactions pin:
//
//   - Every admin response field uses an explicit snake_case json tag —
//     ADR-000941 resolves the PascalCase-vs-tag question ADR-000765 §3
//     deferred, in favor of explicit tags matching altctl's decode
//     structs (cmd/home_retention.go, home_storage.go, home_snapshot.go).
//   - List-shaped responses are wrapped in a named envelope
//     (`{"logs": [...]}`, `{"partitions": [...]}`, `{"tables": [...]}`,
//     `{"snapshots": [...]}`), not bare top-level arrays.
//   - EventSeqBoundary is a positive integer — append-first invariant.
//   - SnapshotID is a UUIDv4.
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
				"snapshot_id":     matchers.Regex("11111111-2222-3333-4444-555555555555", "^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$"),
				"status":          matchers.Like("valid"),
				"items_row_count": matchers.Like(1),
				"snapshot_at":     matchers.Like("2026-04-23T00:00:00Z"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp struct {
				SnapshotID    string `json:"snapshot_id"`
				Status        string `json:"status"`
				ItemsRowCount int    `json:"items_row_count"`
				SnapshotAt    string `json:"snapshot_at"`
			}
			if err := c.Post(context.Background(), "/admin/snapshots/create", map[string]any{}, &resp); err != nil {
				return err
			}
			assert.NotEmpty(t, resp.SnapshotID)
			assert.Equal(t, "valid", resp.Status)
			assert.NotEmpty(t, resp.SnapshotAt)
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
				"snapshot_id":        matchers.Regex("11111111-2222-3333-4444-555555555555", "^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$"),
				"status":             matchers.Like("valid"),
				"projection_version": matchers.Like(1),
				"event_seq_boundary": matchers.Like(1),
				"items_row_count":    matchers.Like(1),
				"digest_row_count":   matchers.Like(1),
				"recall_row_count":   matchers.Like(1),
				"snapshot_at":        matchers.Like("2026-04-23T00:00:00Z"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp struct {
				SnapshotID        string `json:"snapshot_id"`
				Status            string `json:"status"`
				ProjectionVersion int    `json:"projection_version"`
				EventSeqBoundary  int64  `json:"event_seq_boundary"`
				ItemsRowCount     int    `json:"items_row_count"`
				DigestRowCount    int    `json:"digest_row_count"`
				RecallRowCount    int    `json:"recall_row_count"`
				SnapshotAt        string `json:"snapshot_at"`
			}
			if err := c.Get(context.Background(), "/admin/snapshots/latest", &resp); err != nil {
				return err
			}
			assert.Greater(t, resp.EventSeqBoundary, int64(0), "EventSeqBoundary must be > 0 (append-first invariant)")
			return nil
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
			Body: matchers.MapMatcher{
				"snapshots": matchers.EachLike(
					matchers.MapMatcher{
						"snapshot_id":        matchers.Regex("11111111-2222-3333-4444-555555555555", "^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$"),
						"status":             matchers.Like("valid"),
						"projection_version": matchers.Like(1),
						"event_seq_boundary": matchers.Like(1),
						"items_row_count":    matchers.Like(1),
						"snapshot_at":        matchers.Like("2026-04-23T00:00:00Z"),
					},
					1,
				),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp struct {
				Snapshots []struct {
					SnapshotID        string `json:"snapshot_id"`
					Status            string `json:"status"`
					ProjectionVersion int    `json:"projection_version"`
					EventSeqBoundary  int64  `json:"event_seq_boundary"`
					ItemsRowCount     int    `json:"items_row_count"`
					SnapshotAt        string `json:"snapshot_at"`
				} `json:"snapshots"`
			}
			if err := c.Get(context.Background(), "/admin/snapshots/list", &resp); err != nil {
				return err
			}
			require.NotEmpty(t, resp.Snapshots)
			assert.NotEmpty(t, resp.Snapshots[0].SnapshotID)
			return nil
		})
	require.NoError(t, err)
}

func TestRetentionStatus(t *testing.T) {
	mp := newPact(t)

	err := mp.
		AddInteraction().
		Given("at least one retention log entry exists").
		UponReceiving("a retention status query").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/admin/retention/status"),
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"logs": matchers.EachLike(
					matchers.MapMatcher{
						"log_id":           matchers.Regex("11111111-2222-3333-4444-555555555555", "^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$"),
						"action":           matchers.Like("export"),
						"target_table":     matchers.Like("knowledge_events"),
						"target_partition": matchers.Like("knowledge_events_y2025m01"),
						"rows_affected":    matchers.Like(1),
						"dry_run":          matchers.Like(false),
						"status":           matchers.Like("success"),
						"run_at":           matchers.Like("2026-04-23T00:00:00Z"),
					},
					1,
				),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp struct {
				Logs []struct {
					LogID           string `json:"log_id"`
					Action          string `json:"action"`
					TargetTable     string `json:"target_table"`
					TargetPartition string `json:"target_partition"`
					RowsAffected    int64  `json:"rows_affected"`
					DryRun          bool   `json:"dry_run"`
					Status          string `json:"status"`
					RunAt           string `json:"run_at"`
				} `json:"logs"`
			}
			if err := c.Get(context.Background(), "/admin/retention/status", &resp); err != nil {
				return err
			}
			require.NotEmpty(t, resp.Logs)
			assert.NotEmpty(t, resp.Logs[0].Action)
			return nil
		})
	require.NoError(t, err)
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
			Body: matchers.MapMatcher{
				"partitions": matchers.EachLike(
					matchers.MapMatcher{
						"table_name":     matchers.Like("knowledge_events"),
						"partition_name": matchers.Like("knowledge_events_y2025m01"),
						"range_start":    matchers.Like("2025-01-01T00:00:00Z"),
						"range_end":      matchers.Like("2025-02-01T00:00:00Z"),
						"row_count":      matchers.Like(1),
						"size_bytes":     matchers.Like(1),
					},
					1,
				),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp struct {
				Partitions []struct {
					TableName     string `json:"table_name"`
					PartitionName string `json:"partition_name"`
					RangeStart    string `json:"range_start"`
					RangeEnd      string `json:"range_end"`
					RowCount      int64  `json:"row_count"`
					SizeBytes     int64  `json:"size_bytes"`
				} `json:"partitions"`
			}
			if err := c.Get(context.Background(), "/admin/retention/eligible", &resp); err != nil {
				return err
			}
			require.NotEmpty(t, resp.Partitions)
			assert.NotEmpty(t, resp.Partitions[0].TableName)
			return nil
		})
	require.NoError(t, err)
}

func TestStorageStats(t *testing.T) {
	mp := newPact(t)

	err := mp.
		AddInteraction().
		Given("storage stats are available").
		UponReceiving("a storage stats query").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/admin/storage/stats"),
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"tables": matchers.EachLike(
					matchers.MapMatcher{
						"name":       matchers.Like("knowledge_events"),
						"total_size": matchers.Like("128 kB"),
						"table_size": matchers.Like("96 kB"),
						"index_size": matchers.Like("32 kB"),
						"row_count":  matchers.Like(1),
					},
					1,
				),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			c := clientFor(config)
			var resp struct {
				Tables []struct {
					Name      string `json:"name"`
					TotalSize string `json:"total_size"`
					TableSize string `json:"table_size"`
					IndexSize string `json:"index_size"`
					RowCount  int64  `json:"row_count"`
				} `json:"tables"`
			}
			if err := c.Get(context.Background(), "/admin/storage/stats", &resp); err != nil {
				return err
			}
			require.NotEmpty(t, resp.Tables)
			assert.NotEmpty(t, resp.Tables[0].Name)
			return nil
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
