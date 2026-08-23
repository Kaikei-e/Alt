package consumer

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestSendToDLQ_DoesNotDuplicateOnAckFailure reproduces ESC-6: when the DLQ
// XADD lands but the subsequent XACK is refused, the message stays in the PEL
// and the next reclaim sweep calls sendToDLQ again. Without an "already
// written, ACK still owed" record, that re-XADDs the same payload every sweep,
// filling the DLQ the cap exists to bound. sendToDLQ must copy exactly once and
// then only retry the XACK.
func TestSendToDLQ_DoesNotDuplicateOnAckFailure(t *testing.T) {
	srv := miniredis.RunT(t)
	ctx := context.Background()

	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer func() { _ = client.Close() }()

	const stream = "alt:events:articles"
	const group = "pre-processor-group"
	const dlq = "alt:events:articles:dlq"

	// Force XACK to fail while the DLQ XADD (a different key) still succeeds:
	// set the source-stream key to a plain string so XACK on it returns
	// WRONGTYPE, exercising the "XADD ok, XACK failed" path.
	if err := client.Set(ctx, stream, "not-a-stream", 0).Err(); err != nil {
		t.Fatalf("seed WRONGTYPE key: %v", err)
	}

	c := &Consumer{
		client: client,
		logger: newQuietLogger(),
		config: Config{
			StreamKey:     stream,
			GroupName:     group,
			DLQStreamKey:  dlq,
			MaxDeliveries: 2,
		},
	}

	msg := redis.XMessage{ID: "1-0", Values: map[string]interface{}{"event_id": "e1", "payload": "{}"}}

	c.sendToDLQ(ctx, msg, 3)
	c.sendToDLQ(ctx, msg, 3) // simulates the next sweep re-reaping the still-pending message

	n, err := client.XLen(ctx, dlq).Result()
	if err != nil {
		t.Fatalf("XLen DLQ: %v", err)
	}
	if n != 1 {
		t.Fatalf("DLQ has %d entries after two sweeps of one poison message, want 1 (duplicated on XACK failure)", n)
	}
	if !c.dlqWritePending(msg.ID) {
		t.Fatal("message with a durable DLQ copy but failed XACK should be remembered as awaiting-ack")
	}

	// Now make the XACK able to succeed: drop the WRONGTYPE key and provision a
	// real stream + group. A later sweep must clear the awaiting-ack record and
	// must not add a third DLQ copy.
	if err := client.Del(ctx, stream).Err(); err != nil {
		t.Fatalf("Del WRONGTYPE key: %v", err)
	}
	if err := client.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}
	c.sendToDLQ(ctx, msg, 3)

	n, err = client.XLen(ctx, dlq).Result()
	if err != nil {
		t.Fatalf("XLen DLQ (post-ack): %v", err)
	}
	if n != 1 {
		t.Fatalf("DLQ has %d entries after the XACK finally succeeded, want 1", n)
	}
	if c.dlqWritePending(msg.ID) {
		t.Fatal("awaiting-ack record should be cleared once the XACK lands")
	}
}
