package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestReclaimPending_BoundsDLQStreamLength reproduces the finding: the DLQ
// XADD carried no MAXLEN, and nothing else trims that stream -- mq-hub's
// periodic XTRIM pass only walked the four live streams in
// domain.AllStreamKeys() and no service consumes a DLQ. redis-streams runs
// maxmemory 1gb under noeviction where XADD is denyoom, so an unbounded DLQ
// carrying whole original payloads eventually rejects every producer's
// publish -- and publish-time trimming cannot shrink anything back afterwards,
// because it only runs as part of a successful XADD (compose/mq.yaml records
// that self-locking condition).
//
// The cap is driven through ConfigFromEnv rather than a struct literal so the
// test covers the path the container actually boots on (bootstrap/app.go).
func TestReclaimPending_BoundsDLQStreamLength(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx := context.Background()
	seedClient := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer func() { _ = seedClient.Close() }()

	if err := seedClient.XGroupCreateMkStream(ctx, reclaimTestStream, reclaimTestGroup, "0").Err(); err != nil {
		t.Fatalf("seed XGroupCreateMkStream: %v", err)
	}

	const poisonCount = 10
	for i := 0; i < poisonCount; i++ {
		if err := seedClient.XAdd(ctx, &redis.XAddArgs{
			Stream: reclaimTestStream,
			Values: map[string]interface{}{
				"event_id": fmt.Sprintf("poison-%d", i),
				"payload":  `{"article_id":"abc"}`,
			},
		}).Err(); err != nil {
			t.Fatalf("seed poison XAdd %d: %v", i, err)
		}
	}

	// Deliver to a ghost consumer that never ACKs, so every entry is already
	// at one delivery when the reclaim sweep claims it.
	if _, err := seedClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    reclaimTestGroup,
		Consumer: "ghost-consumer",
		Streams:  []string{reclaimTestStream, ">"},
		Count:    poisonCount,
	}).Result(); err != nil {
		t.Fatalf("seed XReadGroup: %v", err)
	}

	claimIdleTime := 10 * time.Millisecond
	const dlqMaxLen = 3

	t.Setenv("CONSUMER_DLQ_MAX_LEN", fmt.Sprintf("%d", dlqMaxLen))

	cfg := ConfigFromEnv()
	cfg.RedisURL = fmt.Sprintf("redis://%s", srv.Addr())
	cfg.GroupName = reclaimTestGroup
	cfg.ConsumerName = "consumer-a"
	cfg.StreamKey = reclaimTestStream
	cfg.BatchSize = poisonCount
	cfg.ClaimIdleTime = claimIdleTime
	cfg.MaxDeliveries = 1
	cfg.Enabled = true

	handler := &recordingHandler{err: fmt.Errorf("poison message: always fails")}

	c, err := NewConsumer(cfg, handler, newQuietLogger())
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer c.Stop()

	srv.SetTime(time.Now().Add(claimIdleTime + time.Second))

	if err := c.reclaimPending(ctx); err != nil {
		t.Fatalf("reclaimPending: %v", err)
	}

	// miniredis trims exactly and ignores the "~" modifier; real Redis keeps
	// at least the cap. Either way the point is that a cap is attached at all,
	// so the DLQ cannot grow without bound.
	dlqLen, err := seedClient.XLen(ctx, cfg.DLQStreamKey).Result()
	if err != nil {
		t.Fatalf("XLen DLQ: %v", err)
	}
	if dlqLen == 0 {
		t.Fatalf("DLQ stream is empty: the %d poison messages were never routed", poisonCount)
	}
	if dlqLen > dlqMaxLen {
		t.Fatalf("DLQ stream length = %d after routing %d poison messages, want <= %d (the XADD carries no MAXLEN, so nothing bounds this stream)", dlqLen, poisonCount, dlqMaxLen)
	}
}

// TestDLQMaxLen_UnsetConfigStillBounded pins the fallback: Config is also
// built as a struct literal, so a field the call site does not know about
// stays zero. A zero cap must resolve to the package default rather than to
// "unbounded" -- an unbounded DLQ is the failure mode the cap exists to
// prevent, so it must not be reachable by forgetting to set a field.
func TestDLQMaxLen_UnsetConfigStillBounded(t *testing.T) {
	t.Parallel()

	if got := (Config{}).effectiveDLQMaxLen(); got != defaultDLQMaxLen {
		t.Fatalf("Config{}.effectiveDLQMaxLen() = %d, want %d", got, defaultDLQMaxLen)
	}
	if got := (Config{DLQMaxLen: 42}).effectiveDLQMaxLen(); got != 42 {
		t.Fatalf("Config{DLQMaxLen: 42}.effectiveDLQMaxLen() = %d, want 42", got)
	}
	if cfg := DefaultConfig(); cfg.DLQMaxLen <= 0 {
		t.Fatalf("DefaultConfig().DLQMaxLen = %d, want a positive cap", cfg.DLQMaxLen)
	}
}

// recordCapture collects slog records so a test can assert on the fields a
// log line actually carries. Records are appended under a mutex because the
// loops Start() spawns log concurrently with the assertions.
type recordCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordCapture) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordCapture) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordCapture) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *recordCapture) WithGroup(string) slog.Handler { return h }

// attr returns the value logged under key by the first record with the given
// message, plus every key that record did carry, so a failure can report what
// was logged instead of just what was missing.
func (h *recordCapture) attr(msg, key string) (slog.Value, []string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, r := range h.records {
		if r.Message != msg {
			continue
		}
		var (
			value slog.Value
			found bool
			keys  []string
		)
		r.Attrs(func(a slog.Attr) bool {
			keys = append(keys, a.Key)
			if a.Key == key {
				value, found = a.Value, true
			}
			return true
		})
		return value, keys, found
	}
	return slog.Value{}, nil, false
}

// TestConsumer_Start_LogsEffectiveDLQMaxLen pins the operator-visible half of
// the DLQ cap. effectiveDLQMaxLen resolving a zero DLQMaxLen to the package
// default is only defensible because the value it picked is loud at startup:
// without it in the consumer-start log, "an operator configured 10000" and
// "the field was never wired" look identical from the outside, which is the
// silent-fallback state CLAUDE.md rule 8 forbids. pre-processor's consumer
// logs the same field -- both halves of the cap must report it.
//
// The cap is deliberately left unset here: the log must carry the value XADD
// will actually use, not the raw zero the field holds.
func TestConsumer_Start_LogsEffectiveDLQMaxLen(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seedClient := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer func() { _ = seedClient.Close() }()

	if err := seedClient.XGroupCreateMkStream(ctx, reclaimTestStream, reclaimTestGroup, "0").Err(); err != nil {
		t.Fatalf("seed XGroupCreateMkStream: %v", err)
	}

	cfg := Config{
		RedisURL:       fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:      reclaimTestGroup,
		ConsumerName:   "consumer-a",
		StreamKey:      reclaimTestStream,
		BatchSize:      10,
		BlockTimeout:   100 * time.Millisecond,
		ClaimIdleTime:  time.Minute,
		ReaperInterval: time.Minute,
		DLQStreamKey:   "alt:events:articles:dlq",
		MaxDeliveries:  5,
		Enabled:        true,
	}

	capture := &recordCapture{}

	c, err := NewConsumer(cfg, &recordingHandler{}, slog.New(capture))
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()

	value, keys, found := capture.attr("starting consumer", "dlq_max_len")
	if !found {
		t.Fatalf("consumer-start log carries no \"dlq_max_len\" field (logged keys: %v); an operator cannot tell a configured cap from an unwired one", keys)
	}
	if got := value.Int64(); got != cfg.effectiveDLQMaxLen() {
		t.Fatalf("consumer-start log dlq_max_len = %d, want %d (the effective cap, not the raw field)", got, cfg.effectiveDLQMaxLen())
	}
}
