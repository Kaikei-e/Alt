//go:build contract

// Pact CDC: alt-backend → alt-data-hub, Web Push storage.
//
// Two groups with one provider. The subscription procedures back
// alt.push.v1.PushService, which cmd/backend serves to the browser; the
// delivery-queue procedures back the Web Push dispatcher.
//
// What these interactions pin, beyond "the procedure exists":
//
//   - user_id is a field on every subscription procedure. alt-data-hub sees a
//     peer certificate naming alt-backend, so an implementation that scoped
//     these to the transport identity would read one service account's devices
//     for every caller and still answer 200.
//   - protoJSON omits false. A preferences update that turns the daily digest
//     off travels as an *absent* key, not as `false`, so a provider reading
//     absence as "leave it alone" would silently refuse every opt-out. The
//     UpdatePushSubscriptionPreferences interaction below sends exactly that
//     shape on purpose.
//   - int64 is a JSON string and int32 is a JSON number. leaseSeconds is
//     int64, statusCode and attempts are int32; a hand-drawn body that got
//     either wrong would pass locally and fail on the wire.
//   - The claim answers rows that are already SENDING, carrying the endpoint
//     and key material. The lease and the state change are one provider-side
//     statement, so a consumer must never observe a claimed row as PENDING,
//     and a claim that omitted the key material would hand the dispatcher a
//     row it cannot send.
//   - EnqueueNotification is a fan-out. One notification becomes one row per
//     device, so the response is a count and not an id — a consumer written
//     against a single-row enqueue would silently treat "three devices" as
//     "one delivery".
package contract

import (
	"context"
	"fmt"
	"testing"
	"time"

	"alt/domain"
	"alt/shared/gateway/datahub_gateway"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The endpoint below is a documented example host, not a real push service.
// A pact file is committed to the repository and published to the Broker, so a
// genuine endpoint here would be a capability URL in version control.
const (
	pushUserID   = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	pushEndpoint = "https://push.example.com/subscription/AAAA-BBBB-CCCC"
)

// ---------------------------------------------------------------------------
// Subscriptions
// ---------------------------------------------------------------------------

func TestUpsertPushSubscriptionContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts push subscription writes").
		UponReceiving("an UpsertPushSubscription request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/UpsertPushSubscription"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"subscription": map[string]interface{}{
					"userId":   pushUserID,
					"endpoint": pushEndpoint,
					"p256dh":   "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw",
					"auth":     "tBHItJI5svbpez7KI4CCXg",
					// All four true, so every field is present on the wire.
					// A preferences block with a false in it would omit that
					// key entirely — see the update interaction below.
					"preferences": map[string]interface{}{
						"summaryReady":       true,
						"acolyteReportReady": true,
						"recapReady":         true,
						"todayEntranceReady": true,
					},
					"vapidKeyFingerprint": "b0a1c2d3e4f5",
				},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"created": matchers.Like(true)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushSubscriptionGateway(newDataHubServiceClient(config))

			created, err := gw.Upsert(context.Background(), domain.PushSubscription{
				UserID:   pushUserID,
				Endpoint: pushEndpoint,
				P256dh:   "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw",
				Auth:     "tBHItJI5svbpez7KI4CCXg",
				Preferences: domain.NotificationPreferences{
					SummaryReady:       true,
					AcolyteReportReady: true,
					RecapReady:         true,
					TodayEntranceReady: true,
				},
				VAPIDKeyFingerprint: "b0a1c2d3e4f5",
			})
			if err != nil {
				return fmt.Errorf("Upsert failed: %w", err)
			}
			assert.True(t, created, "a first registration must report created")
			return nil
		})
	require.NoError(t, err)
}

func TestGetPushSubscriptionContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a push subscription for the user").
		UponReceiving("a GetPushSubscription request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetPushSubscription"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				// Both fields, always. The endpoint alone identifies the row;
				// the user scoping is what stops a leaked endpoint from
				// reading someone else's settings.
				"userId":   pushUserID,
				"endpoint": pushEndpoint,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"subscription": matchers.Like(map[string]interface{}{
					"userId":   matchers.Regex(pushUserID, uuidLikePattern),
					"endpoint": matchers.Like(pushEndpoint),
					"p256dh":   matchers.Like("BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw"),
					"auth":     matchers.Like("tBHItJI5svbpez7KI4CCXg"),
					"preferences": matchers.Like(map[string]interface{}{
						"summaryReady": matchers.Like(true),
						"recapReady":   matchers.Like(true),
					}),
					"vapidKeyFingerprint": matchers.Like("b0a1c2d3e4f5"),
					"createdAt":           matchers.Like("2026-08-01T00:00:00Z"),
					"updatedAt":           matchers.Like("2026-08-01T00:00:00Z"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushSubscriptionGateway(newDataHubServiceClient(config))

			sub, err := gw.Get(context.Background(), pushUserID, pushEndpoint)
			if err != nil {
				return fmt.Errorf("Get failed: %w", err)
			}
			require.NotNil(t, sub)
			assert.Equal(t, pushEndpoint, sub.Endpoint)
			assert.True(t, sub.Preferences.SummaryReady)
			assert.True(t, sub.Preferences.RecapReady)
			// Absent on the wire, therefore off. Not "unknown".
			assert.False(t, sub.Preferences.TodayEntranceReady)
			return nil
		})
	require.NoError(t, err)
}

// TestGetPushSubscriptionMissContract pins the answer for an endpoint this
// user has not registered: an empty envelope, not a 404 and not a zero-valued
// subscription. The settings page reads the absence as "not subscribed on this
// device" and offers the subscribe button.
func TestGetPushSubscriptionMissContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no push subscription for the endpoint").
		UponReceiving("a GetPushSubscription request from alt-backend for an unregistered endpoint").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetPushSubscription"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId":   pushUserID,
				"endpoint": "https://push.example.com/subscription/UNKNOWN",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushSubscriptionGateway(newDataHubServiceClient(config))

			sub, err := gw.Get(context.Background(), pushUserID,
				"https://push.example.com/subscription/UNKNOWN")
			if err != nil {
				return fmt.Errorf("Get failed: %w", err)
			}
			assert.Nil(t, sub, "an unregistered endpoint is a nil subscription, not an error")
			return nil
		})
	require.NoError(t, err)
}

// TestUpdatePushSubscriptionPreferencesContract sends the shape that breaks a
// naive provider: three kinds on, the daily digest off. protoJSON omits the
// false, so `todayEntranceReady` is simply absent — a provider that treated an
// absent key as "leave the column alone" would accept the request, answer
// updated=true, and never turn the digest off.
func TestUpdatePushSubscriptionPreferencesContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a push subscription for the user").
		UponReceiving("an UpdatePushSubscriptionPreferences request from alt-backend turning the digest off").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/UpdatePushSubscriptionPreferences"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId":   pushUserID,
				"endpoint": pushEndpoint,
				"preferences": map[string]interface{}{
					"summaryReady":       true,
					"acolyteReportReady": true,
					"recapReady":         true,
				},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"updated": matchers.Like(true)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushSubscriptionGateway(newDataHubServiceClient(config))

			updated, err := gw.UpdatePreferences(context.Background(), pushUserID, pushEndpoint,
				domain.NotificationPreferences{
					SummaryReady:       true,
					AcolyteReportReady: true,
					RecapReady:         true,
					TodayEntranceReady: false,
				})
			if err != nil {
				return fmt.Errorf("UpdatePreferences failed: %w", err)
			}
			assert.True(t, updated)
			return nil
		})
	require.NoError(t, err)
}

func TestDeletePushSubscriptionContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a push subscription for the user").
		UponReceiving("a DeletePushSubscription request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/DeletePushSubscription"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId":   pushUserID,
				"endpoint": pushEndpoint,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"deleted": matchers.Like(true)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushSubscriptionGateway(newDataHubServiceClient(config))

			deleted, err := gw.Delete(context.Background(), pushUserID, pushEndpoint)
			if err != nil {
				return fmt.Errorf("Delete failed: %w", err)
			}
			assert.True(t, deleted)
			return nil
		})
	require.NoError(t, err)
}

func TestListPushSubscriptionsForUserContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has push subscriptions for the user").
		UponReceiving("a ListPushSubscriptionsForUser request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListPushSubscriptionsForUser"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": pushUserID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"subscriptions": matchers.EachLike(map[string]interface{}{
					"userId":              matchers.Regex(pushUserID, uuidLikePattern),
					"endpoint":            matchers.Like(pushEndpoint),
					"p256dh":              matchers.Like("BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw"),
					"auth":                matchers.Like("tBHItJI5svbpez7KI4CCXg"),
					"vapidKeyFingerprint": matchers.Like("b0a1c2d3e4f5"),
					"createdAt":           matchers.Like("2026-08-01T00:00:00Z"),
					"updatedAt":           matchers.Like("2026-08-01T00:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushSubscriptionGateway(newDataHubServiceClient(config))

			subs, err := gw.ListForUser(context.Background(), pushUserID)
			if err != nil {
				return fmt.Errorf("ListForUser failed: %w", err)
			}
			require.Len(t, subs, 1)
			assert.Equal(t, pushEndpoint, subs[0].Endpoint)
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Push delivery queue
// ---------------------------------------------------------------------------

const (
	deliveryID     = "9c858901-8a57-4791-81fe-4c455b099bc9"
	subscriptionID = "1b4e28ba-2fa1-4d3b-a3f5-ccee1bf27e11"
)

func TestEnqueueNotificationContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	occurredAt := time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)
	expiresAt := occurredAt.Add(24 * time.Hour)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts notification enqueues").
		UponReceiving("an EnqueueNotification request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/EnqueueNotification"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				// Derived from the business fact. A retry of the producing
				// operation must send this same key, which is the whole reason
				// it is not generated at send time.
				"dedupeKey": "recap:7d2a1c34-0a5f-4e51-9b1f-1a2b3c4d5e6f",
				"userId":    pushUserID,
				"kind":      "recap_ready",
				// bytes over protoJSON is base64; this decodes to
				// {"recap_id":"r1"}.
				"payload":    "eyJyZWNhcF9pZCI6InIxIn0=",
				"occurredAt": "2026-08-01T09:30:00Z",
				"expiresAt":  "2026-08-02T09:30:00Z",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				// A count, not an id: one notification fans out to every
				// device the user has registered that still wants this kind.
				"deliveryCount":   matchers.Like(2),
				"supersededCount": matchers.Like(0),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushDeliveryGateway(newDataHubServiceClient(config))

			delivered, superseded, err := gw.Enqueue(context.Background(), domain.NotificationEnqueue{
				DedupeKey:  "recap:7d2a1c34-0a5f-4e51-9b1f-1a2b3c4d5e6f",
				UserID:     pushUserID,
				Kind:       domain.NotificationKindRecapReady,
				Payload:    []byte(`{"recap_id":"r1"}`),
				OccurredAt: occurredAt,
				ExpiresAt:  expiresAt,
			})
			if err != nil {
				return fmt.Errorf("Enqueue failed: %w", err)
			}
			assert.Equal(t, 2, delivered, "the fan-out reaches every device that wants this kind")
			assert.Equal(t, 0, superseded)
			return nil
		})
	require.NoError(t, err)
}

// TestClaimNotificationBatchContract pins the capability the dispatcher rests
// on. The returned rows are already SENDING — the lease and the state change
// are one statement on the provider — so a consumer must never see PENDING
// here, and leaseSeconds travels as a JSON string because it is int64.
func TestClaimNotificationBatchContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has due push deliveries").
		UponReceiving("a ClaimNotificationBatch request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ClaimNotificationBatch"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"lockedBy":     "push-dispatcher-1",
				"limit":        10,
				"leaseSeconds": "60",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"deliveries": matchers.EachLike(map[string]interface{}{
					"id":             matchers.Regex(deliveryID, uuidLikePattern),
					"dedupeKey":      matchers.Like("recap:7d2a1c34-0a5f-4e51-9b1f-1a2b3c4d5e6f"),
					"subscriptionId": matchers.Regex(subscriptionID, uuidLikePattern),
					"userId":         matchers.Regex(pushUserID, uuidLikePattern),
					"kind":           matchers.Like("recap_ready"),
					"payload":        matchers.Like("eyJyZWNhcF9pZCI6InIxIn0="),
					"occurredAt":     matchers.Like("2026-08-01T09:30:00Z"),
					// Matched exactly rather than Like(): the point of the
					// capability is that a claimed row is never observable as
					// pending.
					"state":         matchers.String("NOTIFICATION_STATE_SENDING"),
					"attempts":      matchers.Like(1),
					"nextAttemptAt": matchers.Like("2026-08-01T09:31:00Z"),
					"expiresAt":     matchers.Like("2026-08-02T09:30:00Z"),
					// Without these the dispatcher holds a lease on a row it
					// cannot send.
					"endpoint": matchers.Like(pushEndpoint),
					"p256dh":   matchers.Like("BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkTtF71JbFw"),
					"auth":     matchers.Like("tBHItJI5svbpez7KI4CCXg"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushDeliveryGateway(newDataHubServiceClient(config))

			deliveries, err := gw.ClaimBatch(context.Background(), "push-dispatcher-1", 10, time.Minute)
			if err != nil {
				return fmt.Errorf("ClaimBatch failed: %w", err)
			}
			require.Len(t, deliveries, 1)
			assert.Equal(t, domain.NotificationSending, deliveries[0].State)
			assert.Equal(t, 1, deliveries[0].Attempts)
			assert.Equal(t, pushEndpoint, deliveries[0].Endpoint)
			assert.JSONEq(t, `{"recap_id":"r1"}`, string(deliveries[0].Payload),
				"the base64 payload must survive the wire as the bytes the producer wrote")
			return nil
		})
	require.NoError(t, err)
}

func TestMarkNotificationSentContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a claimed push delivery").
		UponReceiving("a MarkNotificationSent request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/MarkNotificationSent"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"id": deliveryID,
				// int32 is a JSON number, unlike leaseSeconds above.
				"statusCode": 201,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushDeliveryGateway(newDataHubServiceClient(config))
			return gw.MarkSent(context.Background(), deliveryID, 201)
		})
	require.NoError(t, err)
}

// TestReleaseNotificationContract pins the retry path. next_attempt_at is the
// caller's, and it is required: releasing without one would return the row to
// a claim that fires immediately, which is a spin rather than a backoff.
func TestReleaseNotificationContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	nextAttempt := time.Date(2026, 8, 1, 9, 45, 0, 0, time.UTC)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a claimed push delivery").
		UponReceiving("a ReleaseNotification request from alt-backend after a retryable failure").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ReleaseNotification"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"id":            deliveryID,
				"nextAttemptAt": "2026-08-01T09:45:00Z",
				"errorMessage":  "push service refused: 503",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushDeliveryGateway(newDataHubServiceClient(config))
			return gw.Release(context.Background(), deliveryID, nextAttempt, "push service refused: 503")
		})
	require.NoError(t, err)
}

func TestMarkNotificationDeadContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a claimed push delivery").
		UponReceiving("a MarkNotificationDead request from alt-backend after an unrecoverable failure").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/MarkNotificationDead"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"id":           deliveryID,
				"statusCode":   413,
				"errorMessage": "payload too large",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewPushDeliveryGateway(newDataHubServiceClient(config))
			return gw.MarkDead(context.Background(), deliveryID, 413, "payload too large")
		})
	require.NoError(t, err)
}
