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

// reapPending scans the group's pending entries list and forwards messages
// that have exceeded MaxDeliveries to the DLQ stream. Redis Streams has no
// built-in DLQ primitive, so we XADD the original payload plus a failure
// reason to a separate stream, then XACK the original so the main consumer
// stops re-delivering it.
func (c *Consumer) reapPending(ctx context.Context) error {
	if c.config.DLQStreamKey == "" || c.config.MaxDeliveries <= 0 {
		return nil
	}

	pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: c.config.StreamKey,
		Group:  c.config.GroupName,
		Start:  "-",
		End:    "+",
		Count:  int64(c.config.BatchSize * 2),
	}).Result()
	if err != nil {
		return err
	}

	for _, p := range pending {
		if !shouldSendToDLQ(p.RetryCount, c.config.MaxDeliveries) {
			continue
		}

		// Claim the message so we become its owner before moving it.
		claimed, err := c.client.XClaim(ctx, &redis.XClaimArgs{
			Stream:   c.config.StreamKey,
			Group:    c.config.GroupName,
			Consumer: c.config.ConsumerName,
			MinIdle:  0,
			Messages: []string{p.ID},
		}).Result()
		if err != nil || len(claimed) == 0 {
			continue
		}
		msg := claimed[0]

		// XADD original payload + failure context to DLQ.
		values := map[string]any{
			"dlq_reason":        "max_deliveries_exceeded",
			"dlq_delivery_count": p.RetryCount,
			"dlq_original_id":   p.ID,
			"dlq_reaped_at":     time.Now().UTC().Format(time.RFC3339),
		}
		for k, v := range msg.Values {
			values[k] = v
		}
		if err := c.client.XAdd(ctx, &redis.XAddArgs{
			Stream: c.config.DLQStreamKey,
			Values: values,
		}).Err(); err != nil {
			c.logger.Error("failed to write DLQ entry",
				"message_id", p.ID, "error", err)
			continue
		}

		// Acknowledge the original so the group stops re-delivering it.
		if err := c.client.XAck(ctx, c.config.StreamKey, c.config.GroupName, p.ID).Err(); err != nil {
			c.logger.Error("failed to ack DLQ'd message",
				"message_id", p.ID, "error", err)
			continue
		}

		c.logger.Warn("message routed to DLQ",
			"message_id", p.ID,
			"delivery_count", p.RetryCount,
			"dlq_stream", c.config.DLQStreamKey,
		)
	}

	return nil
}

// runReaper periodically invokes reapPending until ctx is cancelled.
func (c *Consumer) runReaper(ctx context.Context) {
	if c.config.ReaperInterval <= 0 {
		return
	}
	ticker := time.NewTicker(c.config.ReaperInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdownChan:
			return
		case <-ticker.C:
			if err := c.reapPending(ctx); err != nil {
				c.logger.Error("pending reaper failed", "error", err)
			}
		}
	}
}
