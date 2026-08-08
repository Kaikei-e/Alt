package backend_api

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"pre-processor/domain"
	datahubv1 "pre-processor/gen/proto/services/datahub/v1"
)

// NotificationForwarder forwards claimed notification_outbox rows to
// alt-data-hub, which fans each one out to the user's devices.
//
// It reuses the same mTLS Connect client as the article/feed/summary
// repositories: alt-data-hub is one peer with one identity, and a second
// client would be a second set of TLS material and timeouts to keep in sync.
type NotificationForwarder struct {
	client *Client
}

// NewNotificationForwarder creates a forwarder over an existing alt-data-hub
// client.
func NewNotificationForwarder(client *Client) *NotificationForwarder {
	return &NotificationForwarder{client: client}
}

// EnqueueNotification forwards one outbox row. Delivery is at-least-once: the
// row's dedupe_key is what makes a repeat harmless, since alt-data-hub is
// idempotent per (dedupe_key, subscription_id).
func (f *NotificationForwarder) EnqueueNotification(ctx context.Context, row domain.NotificationOutboxRow, expiresAt time.Time) error {
	if row.DedupeKey == "" {
		return fmt.Errorf("dedupe key cannot be empty")
	}

	protoReq := &datahubv1.EnqueueNotificationRequest{
		DedupeKey:  row.DedupeKey,
		UserId:     row.UserID,
		Kind:       row.Kind,
		Payload:    row.Payload,
		OccurredAt: timestamppb.New(row.OccurredAt),
		ExpiresAt:  timestamppb.New(expiresAt),
	}

	req := connect.NewRequest(protoReq)
	f.client.addAuth(req)

	if _, err := f.client.client.EnqueueNotification(ctx, req); err != nil {
		return fmt.Errorf("EnqueueNotification: %w", err)
	}
	return nil
}
