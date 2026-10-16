package domain

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// NotificationPreferences is one subscription's per-kind on/off state.
//
// Four booleans rather than a set of enabled kinds because the question the
// settings screen asks is "is this kind on for this device", and a set makes
// "off" and "this build does not know that kind yet" the same absence.
type NotificationPreferences struct {
	SummaryReady       bool
	AcolyteReportReady bool
	RecapReady         bool
	TodayEntranceReady bool
}

// PushSubscription is one device's Web Push registration.
//
// Endpoint is a capability URL: whoever holds it can push to that device. It
// must never be logged, traced, or echoed in an error message — which is why
// nothing in this repository formats a subscription with %+v.
type PushSubscription struct {
	UserID      string
	Endpoint    string
	P256dh      string
	Auth        string
	Preferences NotificationPreferences
	// VAPIDKeyFingerprint records which VAPID keypair this subscription was
	// created under. Rotating the keypair invalidates every existing
	// subscription and there is no server-side migration for that: the
	// dispatcher skips rows created under a retired key, and the browser
	// re-subscribes after comparing GetPushConfig's key against its own.
	VAPIDKeyFingerprint string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	// Zero until the dispatcher has succeeded / failed against this endpoint.
	LastSuccessAt time.Time
	LastFailureAt time.Time
}

// ValidatePushEndpoint verifies that a Web Push subscription endpoint is a
// canonical HTTPS URL pointing at a known browser push service.
//
// Returned errors deliberately never echo the endpoint string because the
// path is a bearer capability token.
func ValidatePushEndpoint(endpoint string) error {
	if strings.TrimSpace(endpoint) == "" {
		return errors.New("push endpoint is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("invalid push endpoint URL")
	}
	if u.Scheme != "https" {
		return errors.New("push endpoint must use https scheme")
	}
	if u.User != nil {
		return errors.New("push endpoint must not contain userinfo")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return errors.New("push endpoint host is required")
	}
	if port := u.Port(); port != "" && port != "443" {
		return errors.New("push endpoint port must be 443")
	}
	if !isAllowedPushHost(host) {
		return errors.New("push endpoint host is not a supported Web Push provider")
	}
	return nil
}

func isAllowedPushHost(host string) bool {
	if host == "fcm.googleapis.com" {
		return true
	}
	for _, suffix := range []string{
		".fcm.googleapis.com",
		".push.services.mozilla.com",
		".notify.windows.com",
		".push.apple.com",
	} {
		if hasValidSubdomainPrefix(host, suffix) {
			return true
		}
	}
	return false
}

func hasValidSubdomainPrefix(host, suffix string) bool {
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	prefix := strings.TrimSuffix(host, suffix)
	return prefix != "" && !strings.HasPrefix(prefix, ".") && !strings.HasSuffix(prefix, ".")
}

// NotificationKind names the notification_outbox kinds the product sends.
// They are the wire values of `kind`, and the daily digest one is load-bearing
// twice over: the partial unique index that supersedes an unsent digest names
// it literally.
const (
	NotificationKindSummaryReady       = "summary_ready"
	NotificationKindAcolyteReportReady = "acolyte_report_ready"
	NotificationKindRecapReady         = "recap_ready"
	NotificationKindTodayEntranceReady = "today_entrance_ready"
)

// NotificationState is a push_deliveries row's position in its state machine.
type NotificationState string

const (
	// NotificationPending is the initial state, written when the notification
	// was fanned out to this device.
	NotificationPending NotificationState = "pending"
	// NotificationSending means a dispatcher holds the lease. The lease is
	// next_attempt_at, so this state is reclaimable rather than sticky.
	NotificationSending NotificationState = "sending"
	// NotificationSent is terminal: the push service accepted it.
	NotificationSent NotificationState = "sent"
	// NotificationDead is terminal: delivery gave up on a failure that will
	// not improve.
	NotificationDead NotificationState = "dead"
	// NotificationExpired is terminal and reached by the claim query itself
	// when a row passes expires_at before anyone sends it, or by a newer daily
	// digest superseding it. No dispatcher decides to expire a delivery.
	NotificationExpired NotificationState = "expired"
)

// IsTerminal reports whether the state ends the row's lifecycle.
func (s NotificationState) IsTerminal() bool {
	return s == NotificationSent || s == NotificationDead || s == NotificationExpired
}

// NotificationEnqueue is one notification to fan out.
//
// A struct rather than six positional arguments because four of the fields are
// strings and two are timestamps, and a transposition between DedupeKey and
// Kind would compile.
type NotificationEnqueue struct {
	// DedupeKey is derived from the business fact ("recap:<job_id>"). A
	// relayed retry must produce the same key.
	DedupeKey string
	// UserID selects whose devices receive it. One enqueue becomes one row per
	// subscription that still wants this kind.
	UserID  string
	Kind    string
	Payload []byte
	// OccurredAt is business time and is never a wall-clock reading taken by
	// the storage layer: it is when the fact happened, which the producer
	// knows and alt-db does not.
	OccurredAt time.Time
	// ExpiresAt bounds how long delivery is still worth attempting. A
	// job-finished ping and a daily digest go stale at very different rates,
	// so it is the producer's judgement rather than a table-wide constant.
	ExpiresAt time.Time
}

// PushDelivery is one claimed row of push_deliveries: one notification bound
// for one device.
//
// Payload stays opaque here for the same reason OutboxEvent's does: the
// producer serialised it beside the fact it describes, and the dispatcher
// renders it. Nothing in between needs to look inside.
//
// Endpoint, P256dh and Auth ride along because a claim that omitted them would
// hand the dispatcher a row it cannot send, and resolving them would be one
// extra lookup per delivery inside the lease window.
type PushDelivery struct {
	ID             string
	DedupeKey      string
	SubscriptionID string
	UserID         string
	Kind           string
	Payload        []byte
	// OccurredAt is business time — when the fact happened, not when the row
	// was written.
	OccurredAt    time.Time
	State         NotificationState
	Attempts      int
	NextAttemptAt time.Time
	ExpiresAt     time.Time
	// Endpoint is a capability URL. Never log it.
	Endpoint string
	P256dh   string
	Auth     string
}
