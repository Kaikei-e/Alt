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
// the PEL. Called from redis_consumer.go's XAUTOCLAIM reclaim loop once a
// message's delivery count (incremented by the claim itself) exceeds
// MaxDeliveries -- see .claude/rules/event-stream-consumer.md.
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
