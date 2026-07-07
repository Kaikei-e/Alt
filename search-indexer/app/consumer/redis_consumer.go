// Package consumer provides Redis Streams consumer for search-indexer.
package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Event represents a domain event from the stream.
type Event struct {
	// MessageID is the Redis Stream message ID.
	MessageID string
	// EventID is the unique event identifier.
	EventID string
	// EventType is the type of event.
	EventType string
	// Source is the service that produced the event.
	Source string
	// CreatedAt is when the event was created.
	CreatedAt time.Time
	// Payload is the event-specific data.
	Payload json.RawMessage
	// Metadata contains additional context.
	Metadata map[string]string
}

// EventHandler processes events from the stream.
type EventHandler interface {
	// HandleEvent processes a single event.
	HandleEvent(ctx context.Context, event Event) error
}

// Acknowledger acknowledges Redis Stream message IDs once their processing
// side effect is durable. Buffering handlers (e.g. IndexEventHandler) must
// defer XAck until a batch flush confirms the underlying write succeeded --
// see .claude/rules/event-stream-consumer.md ("ACK after durable write").
type Acknowledger interface {
	Ack(ctx context.Context, messageIDs ...string) error
}

// AckSetter is implemented by handlers that need an Acknowledger injected
// after construction (buffering handlers only -- see IndexEventHandler).
// NewConsumer wires it automatically so the ack path can never be left
// accidentally unwired.
type AckSetter interface {
	SetAcker(Acknowledger)
}

// Consumer consumes events from Redis Streams.
type Consumer struct {
	client       *redis.Client
	config       Config
	handler      EventHandler
	logger       *slog.Logger
	shutdownChan chan struct{}
}

// NewConsumer creates a new Redis Streams consumer.
func NewConsumer(config Config, handler EventHandler, logger *slog.Logger) (*Consumer, error) {
	if !config.Enabled {
		return &Consumer{config: config, logger: logger}, nil
	}

	opts, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)

	if logger == nil {
		logger = slog.Default()
	}

	c := &Consumer{
		client:       client,
		config:       config,
		handler:      handler,
		logger:       logger,
		shutdownChan: make(chan struct{}),
	}

	if setter, ok := handler.(AckSetter); ok {
		setter.SetAcker(c)
	}

	return c, nil
}

// Ack acknowledges one or more message IDs against this consumer's
// stream/group.
func (c *Consumer) Ack(ctx context.Context, messageIDs ...string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	return c.client.XAck(ctx, c.config.StreamKey, c.config.GroupName, messageIDs...).Err()
}

// Start begins consuming events from the stream.
func (c *Consumer) Start(ctx context.Context) error {
	if !c.config.Enabled {
		c.logger.Info("consumer disabled, not starting")
		return nil
	}

	// Ensure consumer group exists
	if err := c.ensureConsumerGroup(ctx); err != nil {
		return err
	}

	c.logger.Info("starting consumer",
		"stream", c.config.StreamKey,
		"group", c.config.GroupName,
		"consumer", c.config.ConsumerName,
		"dlq_stream", c.config.DLQStreamKey,
		"max_deliveries", c.config.MaxDeliveries,
	)

	go c.consumeLoop(ctx)
	go c.reclaimLoop(ctx)
	return nil
}

// StopIntake halts the consume and reclaim loops without closing the
// underlying Redis client, so a handler's in-flight flush/ack calls made
// during shutdown (see IndexEventHandler.Stop) can still complete. Callers
// that need the full shutdown sequence should call StopIntake, let any
// handler flush, then call Close -- see
// .claude/rules/event-stream-consumer.md shutdown ordering.
func (c *Consumer) StopIntake() {
	if c.shutdownChan != nil {
		close(c.shutdownChan)
	}
}

// Close closes the underlying Redis client. Call after StopIntake (and
// after any handler has finished flushing/acking pending work).
func (c *Consumer) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// Stop performs the full shutdown: halts intake and closes the client. It
// is a convenience for callers that don't need to interleave handler flush
// between the two steps.
func (c *Consumer) Stop() {
	c.StopIntake()
	c.Close()
}

// IsEnabled returns true if the consumer is enabled.
func (c *Consumer) IsEnabled() bool {
	return c.config.Enabled
}

// ensureConsumerGroup creates the consumer group if it doesn't exist.
func (c *Consumer) ensureConsumerGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, c.config.StreamKey, c.config.GroupName, "0").Err()
	if err != nil {
		// Ignore BUSYGROUP error, it means the group already exists
		if err.Error() == "BUSYGROUP Consumer Group name already exists" {
			return nil
		}
		return err
	}
	return nil
}

// consumeLoop continuously reads and processes events.
func (c *Consumer) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("consumer context cancelled, stopping")
			return
		case <-c.shutdownChan:
			c.logger.Info("consumer shutdown requested, stopping")
			return
		default:
			if err := c.readAndProcess(ctx); err != nil {
				c.logger.Error("error processing events", "error", err)
				time.Sleep(time.Second) // Back off on error
			}
		}
	}
}

// readAndProcess reads new messages from the stream and processes them.
func (c *Consumer) readAndProcess(ctx context.Context) error {
	streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.config.GroupName,
		Consumer: c.config.ConsumerName,
		Streams:  []string{c.config.StreamKey, ">"},
		Count:    c.config.BatchSize,
		Block:    c.config.BlockTimeout,
	}).Result()

	if err == redis.Nil {
		// No messages available
		return nil
	}
	if err != nil {
		return err
	}

	for _, stream := range streams {
		c.processMessages(ctx, stream.Messages)
	}

	return nil
}

// processMessages runs the handler over each message. It intentionally does
// NOT XAck on a nil return: IndexEventHandler buffers article IDs and only
// writes to Meilisearch on a later batch flush, so "HandleEvent returned
// nil" only means "durably buffered for this in-process batch", not "the
// underlying write is durable". The handler XAcks via its injected
// Acknowledger once flush() confirms the write succeeded, and leaves the
// message un-ACKed on flush failure so the reclaim loop retries it -- see
// .claude/rules/event-stream-consumer.md ("ACK after durable write").
func (c *Consumer) processMessages(ctx context.Context, messages []redis.XMessage) {
	for _, message := range messages {
		event := c.parseEvent(message)

		if err := c.handler.HandleEvent(ctx, event); err != nil {
			c.logger.Error("failed to process event",
				"message_id", message.ID,
				"event_type", event.EventType,
				"error", err,
			)
			// Don't ACK failed messages; they'll be retried by the reclaim loop.
		}
	}
}

// reclaimLoop periodically sweeps the stream's pending entries list (PEL)
// via XAUTOCLAIM, reassigning to this consumer any message that has been
// idle (unacknowledged) for longer than ClaimIdleTime. Without this loop,
// messages left in the PEL by a crashed consumer are never redelivered --
// ClaimIdleTime would be dead configuration. See
// .claude/rules/event-stream-consumer.md.
func (c *Consumer) reclaimLoop(ctx context.Context) {
	interval := c.config.ReaperInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("reclaim loop context cancelled, stopping")
			return
		case <-c.shutdownChan:
			c.logger.Info("reclaim loop shutdown requested, stopping")
			return
		case <-ticker.C:
			if err := c.reclaimPending(ctx); err != nil {
				c.logger.Error("error reclaiming pending messages", "error", err)
			}
		}
	}
}

// reclaimPending runs a full XAUTOCLAIM cursor sweep -- looping until Redis
// returns the "0-0" cursor -- claiming every pending entry idle for longer
// than ClaimIdleTime. Claiming increments each message's delivery counter
// (Redis semantics), so poison messages eventually cross MaxDeliveries and
// get routed to the DLQ instead of cycling through reclaim forever;
// everything else is handed back to the handler exactly like a freshly-read
// message.
func (c *Consumer) reclaimPending(ctx context.Context) error {
	cursor := "0-0"
	for {
		messages, next, err := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   c.config.StreamKey,
			Group:    c.config.GroupName,
			Consumer: c.config.ConsumerName,
			MinIdle:  c.config.ClaimIdleTime,
			Start:    cursor,
			Count:    c.config.BatchSize,
		}).Result()
		if err != nil {
			return err
		}

		if len(messages) > 0 {
			c.logger.Info("reclaimed idle pending messages",
				"count", len(messages),
				"min_idle", c.config.ClaimIdleTime,
			)
			c.routeReclaimedMessages(ctx, messages)
		}

		if next == "0-0" {
			return nil
		}
		cursor = next
	}
}

// routeReclaimedMessages splits freshly-reclaimed messages into poison
// messages that have exceeded MaxDeliveries (sent to the DLQ) and the rest,
// which are handed back to the handler for a retry via processMessages. The
// delivery counter used for the DLQ check was already incremented by the
// XAUTOCLAIM call in reclaimPending.
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

// parseEvent converts a Redis Stream message to an Event.
func (c *Consumer) parseEvent(message redis.XMessage) Event {
	event := Event{
		MessageID: message.ID,
		Metadata:  make(map[string]string),
	}

	if v, ok := message.Values["event_id"].(string); ok {
		event.EventID = v
	}
	if v, ok := message.Values["event_type"].(string); ok {
		event.EventType = v
	}
	if v, ok := message.Values["source"].(string); ok {
		event.Source = v
	}
	if v, ok := message.Values["created_at"].(string); ok {
		event.CreatedAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := message.Values["payload"].(string); ok {
		event.Payload = json.RawMessage(v)
	}
	if v, ok := message.Values["metadata"].(string); ok {
		_ = json.Unmarshal([]byte(v), &event.Metadata)
	}

	return event
}
