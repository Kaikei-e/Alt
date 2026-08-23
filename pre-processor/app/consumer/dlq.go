package consumer

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// dlqAwaitingAckMax bounds the set of message IDs whose DLQ copy is durable
// but not yet ACKed. A Redis that keeps refusing XACK must not grow this map
// without limit; evicting the oldest-ish entry costs at most one duplicate DLQ
// copy for that message if it is ever swept again. Matches tag-generator's
// _DLQ_AWAITING_ACK_MAX.
const dlqAwaitingAckMax = 1024

// dlqWritePending reports whether messageID's DLQ copy is already durable and
// only its XACK is still owed.
func (c *Consumer) dlqWritePending(messageID string) bool {
	c.dlqMu.Lock()
	defer c.dlqMu.Unlock()
	_, ok := c.dlqAwaitingAck[messageID]
	return ok
}

// rememberDLQWrite records that messageID's DLQ copy landed so a later sweep
// re-tries only the XACK. It evicts an arbitrary entry when over the bound;
// the awaiting set can only grow while Redis refuses XACKs, and one duplicate
// copy is a cheaper failure than an unbounded map.
func (c *Consumer) rememberDLQWrite(messageID string) {
	c.dlqMu.Lock()
	defer c.dlqMu.Unlock()
	if c.dlqAwaitingAck == nil {
		c.dlqAwaitingAck = make(map[string]struct{})
	}
	c.dlqAwaitingAck[messageID] = struct{}{}
	for len(c.dlqAwaitingAck) > dlqAwaitingAckMax {
		for k := range c.dlqAwaitingAck {
			delete(c.dlqAwaitingAck, k)
			break
		}
	}
}

// forgetDLQWrite drops messageID once its XACK has landed.
func (c *Consumer) forgetDLQWrite(messageID string) {
	c.dlqMu.Lock()
	delete(c.dlqAwaitingAck, messageID)
	c.dlqMu.Unlock()
}

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

	// The copy and the ACK fail independently, so they are handled apart. A
	// refused XADD must leave the entry in the PEL, or the payload is lost the
	// moment Redis is under the memory pressure that refused it. A landed XADD
	// followed by a refused XACK must not be re-copied on the next sweep -- that
	// piles a duplicate of the same payload into the DLQ every reclaim interval,
	// filling the very stream the cap exists to bound. dlqAwaitingAck remembers
	// "written but not yet ACKed" so a re-sweep only retries the XACK.
	if !c.dlqWritePending(message.ID) {
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
			// Don't ACK -- a later sweep dead-letters it once Redis is writable.
			return
		}
		c.rememberDLQWrite(message.ID)
	}

	if err := c.client.XAck(ctx, c.config.StreamKey, c.config.GroupName, message.ID).Err(); err != nil {
		c.logger.Error("failed to ack DLQ'd message", "message_id", message.ID, "error", err)
		// The copy is durable; the next sweep owes only the ACK.
		return
	}
	c.forgetDLQWrite(message.ID)

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
