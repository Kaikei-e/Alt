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

	if err := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.config.DLQStreamKey,
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
// call).
func (c *Consumer) deliveryCounts(ctx context.Context, messages []redis.XMessage) map[string]int64 {
	counts := make(map[string]int64, len(messages))
	if len(messages) == 0 {
		return counts
	}

	pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream:   c.config.StreamKey,
		Group:    c.config.GroupName,
		Consumer: c.config.ConsumerName,
		Start:    "-",
		End:      "+",
		Count:    int64(len(messages)) * 2,
	}).Result()
	if err != nil {
		c.logger.Error("failed to look up delivery counts for reclaimed messages", "error", err)
		return counts
	}

	for _, p := range pending {
		counts[p.ID] = p.RetryCount
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
