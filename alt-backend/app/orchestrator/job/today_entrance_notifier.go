package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

const (
	// todayEntranceTickInterval is how often the job looks at the clock, not
	// how often it notifies. JobScheduler is interval-only and runs every job
	// immediately on start, so a 24h interval would fire at container-start
	// time rather than at an hour anyone chose. A short tick plus the hour
	// gate below lands the notification on a wall-clock hour without teaching
	// the scheduler about cron.
	todayEntranceTickInterval = 10 * time.Minute

	// todayEntranceTriggerHourUTC is the UTC hour the digest is worth sending
	// in.
	//
	// The digest is keyed on a UTC calendar day — today_digest_view is
	// (user_id, digest_date) with digest_date derived from occurred_at, and
	// CountNeedToKnowItems windows on published_at >= start-of-day UTC — so
	// the hour has to be a UTC hour. 00:00 UTC would count a day that has just
	// begun and report ~0 items; 23:00 UTC counts a nearly complete day, and
	// lands at 08:00 in the operator's timezone. There is no per-user timezone
	// anywhere in this system and this job does not invent one.
	todayEntranceTriggerHourUTC = 23

	// todayEntranceTTL is how long the trigger is still worth delivering. It
	// is exactly one day because the next day's enqueue supersedes an unsent
	// digest anyway: yesterday's entrance shown this evening is a lie about
	// which day it describes.
	todayEntranceTTL = 24 * time.Hour

	// todayEntranceNavigate is where a tap lands. The notification carries a
	// trigger, not content — no titles and no article text — so the app
	// fetches the entrance itself over an authenticated navigation load.
	todayEntranceNavigate = "/home"
)

// digestAudienceReader is knowledge-sovereign's half: who has a Knowledge Home
// at all, and how much landed in each one today. Both procedures already exist
// on KnowledgeSovereignService; satisfied by *sovereign_client.Client.
type digestAudienceReader interface {
	ListDistinctUserIDs(ctx context.Context) ([]uuid.UUID, error)
	CountNeedToKnowItems(ctx context.Context, userID uuid.UUID, date time.Time) (int, error)
}

// notificationEnqueuer is alt-data-hub's half. Satisfied by
// *datahub_gateway.PushDeliveryGateway — alt-harvester links no database
// driver, so the enqueue travels over mTLS like every other write.
type notificationEnqueuer interface {
	Enqueue(ctx context.Context, in domain.NotificationEnqueue) (delivered, superseded int, err error)
}

// todayEntrancePayload is the whole notification body. Three fields, and the
// count is the only datum allowed through: anything richer would make the push
// a delivery channel for content that belongs behind authentication, and would
// put it on a lock screen.
type todayEntrancePayload struct {
	Kind  string `json:"kind"`
	URL   string `json:"url"`
	Count int    `json:"count"`
}

type todayEntranceNotifier struct {
	digests       digestAudienceReader
	notifications notificationEnqueuer
	// clock is injected so the hour gate is testable without waiting for
	// 23:00. Production reads the wall clock.
	clock func() time.Time
}

// TodayEntranceNotificationJob returns the scheduler function that sends each
// user one "today's entrance is ready" trigger a day.
//
// Both dependencies are wired unconditionally at cmd/harvester's composition
// root — there is no feature flag that legitimately leaves either nil — so a
// nil one can only be a DI wiring bug and must stop the process rather than
// tick a silent no-op every ten minutes (CLAUDE.md rule 8).
func TodayEntranceNotificationJob(digests digestAudienceReader, notifications notificationEnqueuer) func(ctx context.Context) error {
	return todayEntranceNotificationJob(digests, notifications, time.Now)
}

func todayEntranceNotificationJob(
	digests digestAudienceReader,
	notifications notificationEnqueuer,
	clock func() time.Time,
) func(ctx context.Context) error {
	switch {
	case digests == nil:
		panic("today-entrance-notifier: digest audience reader is nil — the daily entrance notification would never be sent while the job logged a successful pass every ten minutes (see .claude/rules/di-wiring.md)")
	case notifications == nil:
		panic("today-entrance-notifier: notification enqueuer is nil — every computed digest would be discarded instead of delivered (see .claude/rules/di-wiring.md)")
	case clock == nil:
		panic("today-entrance-notifier: clock is nil — the trigger hour could never be evaluated (see .claude/rules/di-wiring.md)")
	}

	n := &todayEntranceNotifier{digests: digests, notifications: notifications, clock: clock}
	return n.run
}

// run notifies every user whose entrance has something in it, once, if this
// tick falls in the trigger hour.
//
// Repeats are the schedule's normal behaviour rather than a bug to guard
// against: six ticks land in the hour and a container restart mid-window adds
// more. Every one of them derives the same dedupe_key from the same UTC day,
// and EnqueueNotification is idempotent per device on it, so the second and
// later firings create nothing.
func (n *todayEntranceNotifier) run(ctx context.Context) error {
	now := n.clock().UTC()
	if now.Hour() != todayEntranceTriggerHourUTC {
		return nil
	}

	users, err := n.digests.ListDistinctUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("list digest audience: %w", err)
	}

	var (
		failures  []error
		notified  int
		skipped   int
		delivered int
	)
	for _, userID := range users {
		count, err := n.digests.CountNeedToKnowItems(ctx, userID, now)
		if err != nil {
			// One user's failure must not cost every later user their
			// notification — this is the only one they get today.
			failures = append(failures, fmt.Errorf("count digest items for %s: %w", userID, err))
			continue
		}
		if count == 0 {
			// "0 new" is worse than silence, and it burns the one daily
			// notification the user agreed to.
			skipped++
			continue
		}

		devices, err := n.enqueue(ctx, userID, count, now)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		notified++
		delivered += devices
	}

	slog.InfoContext(ctx, "today-entrance-notifier: completed",
		"digest_date", now.Format(time.DateOnly),
		"audience", len(users),
		"notified", notified,
		"skipped_empty", skipped,
		"device_deliveries", delivered,
		"failures", len(failures))

	return errors.Join(failures...)
}

func (n *todayEntranceNotifier) enqueue(ctx context.Context, userID uuid.UUID, count int, now time.Time) (int, error) {
	payload, err := json.Marshal(todayEntrancePayload{
		Kind:  domain.NotificationKindTodayEntranceReady,
		URL:   todayEntranceNavigate,
		Count: count,
	})
	if err != nil {
		return 0, fmt.Errorf("marshal today entrance payload for %s: %w", userID, err)
	}

	delivered, superseded, err := n.notifications.Enqueue(ctx, domain.NotificationEnqueue{
		// Derived from the business fact — this user's entrance for this UTC
		// day — so every tick inside the window produces the same key and the
		// provider collapses them into one delivery per device.
		DedupeKey:  fmt.Sprintf("digest:%s:%s", userID, now.Format(time.DateOnly)),
		UserID:     userID.String(),
		Kind:       domain.NotificationKindTodayEntranceReady,
		Payload:    payload,
		OccurredAt: now,
		ExpiresAt:  now.Add(todayEntranceTTL),
	})
	if err != nil {
		return 0, fmt.Errorf("enqueue today entrance notification for %s: %w", userID, err)
	}
	if superseded > 0 {
		slog.InfoContext(ctx, "today-entrance-notifier: superseded an undelivered digest",
			"user_id", userID, "superseded", superseded)
	}
	return delivered, nil
}
