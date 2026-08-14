package consumer

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// shouldSendToDLQ reports whether a pending message's delivery count exceeds
// the configured maximum. maxDeliveries == 0 disables DLQ routing entirely.
func shouldSendToDLQ(deliveryCount, maxDeliveries int64) bool {
	if maxDeliveries <= 0 {
		return false
	}
	return deliveryCount > maxDeliveries
}

// sendToDLQ forwards a poison message's payload plus failure context to the
// DLQ stream, then ACKs the original so the group stops redelivering it.
// Redis Streams has no built-in DLQ primitive, so this XADDs the original
// payload to a separate stream and XACKs the source entry to remove it from
// the PEL. Called from reclaimPending's XAUTOCLAIM sweep once a message's
// delivery count (incremented by the claim itself) exceeds MaxDeliveries --
// see .claude/rules/event-stream-consumer.md.
func (c *Consumer) sendToDLQ(ctx context.Context, message redis.XMessage, deliveryCount int64) {
	if c.config.DLQStreamKey == "" {
		return
	}

	values := map[string]any{
		"dlq_reason":         "max_deliveries_exceeded",
		"dlq_delivery_count": deliveryCount,
		"dlq_original_id":    message.ID,
		"dlq_reaped_at":      time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range message.Values {
		values[k] = v
	}

	// The DLQ is capped here or nowhere: mq-hub's periodic XTRIM pass only
	// walks the four live streams in domain.AllStreamKeys(), and no service
	// consumes a DLQ, so its only bound is the one its producer attaches.
	// Unlike mq-hub's live-stream publishes this deliberately omits Mode
	// "ACKED": the DLQ has no consumer group, so restricting trimming to
	// fully-acked entries would trim nothing and leave the cap decorative.
	if err := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.config.DLQStreamKey,
		MaxLen: c.config.effectiveDLQMaxLen(),
		Approx: true,
		Values: values,
	}).Err(); err != nil {
		c.logger.Error("failed to write DLQ entry", "message_id", message.ID, "error", err)
		return
	}

	if err := c.client.XAck(ctx, c.config.StreamKey, c.config.GroupName, message.ID).Err(); err != nil {
		c.logger.Error("failed to ack DLQ'd message", "message_id", message.ID, "error", err)
		return
	}

	c.logger.Warn("message routed to DLQ",
		"message_id", message.ID,
		"delivery_count", deliveryCount,
		"dlq_stream", c.config.DLQStreamKey,
	)
}

// deliveryCounts looks up the current delivery counter for each given
// message (already updated by the XAUTOCLAIM claim that preceded this
// call). It queries per message ID -- Start/End pinned to that exact ID --
// rather than a single Start:"-",End:"+" sweep bounded by a Count derived
// from len(messages): when this consumer's PEL holds more pending entries
// than that Count, the ascending-ID-ordered sweep returns only the earliest
// entries and silently omits any message in this batch whose ID sorts later,
// making its delivery count read as zero regardless of its real value. That
// falsely keeps shouldSendToDLQ from ever firing for it, so every sweep
// re-runs the poison message's handler. See mq-hub/CLAUDE.md rule 5.
func (c *Consumer) deliveryCounts(ctx context.Context, messages []redis.XMessage) map[string]int64 {
	counts := make(map[string]int64, len(messages))

	for _, message := range messages {
		pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream:   c.config.StreamKey,
			Group:    c.config.GroupName,
			Consumer: c.config.ConsumerName,
			Start:    message.ID,
			End:      message.ID,
			Count:    1,
		}).Result()
		if err != nil {
			c.logger.Error("failed to look up delivery count for reclaimed message",
				"message_id", message.ID,
				"error", err,
			)
			continue
		}
		for _, p := range pending {
			counts[p.ID] = p.RetryCount
		}
	}
	return counts
}

// routeReclaimedMessages splits freshly-reclaimed messages into poison
// messages that have exceeded MaxDeliveries (sent to the DLQ) and the rest,
// which are handed back to processMessages for a retry. The delivery
// counter used for the DLQ check was already incremented by the XAUTOCLAIM
// call in reclaimPending.
func (c *Consumer) routeReclaimedMessages(ctx context.Context, messages []redis.XMessage) {
	retryCounts := c.deliveryCounts(ctx, messages)

	retryable := make([]redis.XMessage, 0, len(messages))
	for _, message := range messages {
		count := retryCounts[message.ID]
		if shouldSendToDLQ(count, c.config.MaxDeliveries) {
			c.sendToDLQ(ctx, message, count)
			continue
		}
		retryable = append(retryable, message)
	}

	c.processMessages(ctx, retryable)
}
