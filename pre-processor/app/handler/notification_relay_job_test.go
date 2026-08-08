package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pre-processor/domain"
	"pre-processor/metrics"
	"pre-processor/repository"
	"pre-processor/service"
)

// TestStartNotificationRelayJob_FailsWhenUnwired is the guard against the
// failure mode ADR-000928 keeps reproducing: a component that exists, compiles
// and is unit-tested, but was never handed to the composition root. Skipping
// the job silently would make "the relay is off" indistinguishable from "the
// relay was forgotten".
func TestStartNotificationRelayJob_FailsWhenUnwired(t *testing.T) {
	h := NewJobHandler(context.Background(), nil, nil, nil, nil, nil, nil, 10, testJobHandlerLogger())

	err := h.StartNotificationRelayJob(context.Background())

	require.Error(t, err)
}

func TestStartNotificationRelayJob_RegistersTheRelay(t *testing.T) {
	log := testJobHandlerLogger()
	relay, err := service.NewNotificationRelay(
		repository.NewNotificationOutboxRepository(nil, log),
		&noopForwarder{},
		metrics.NewOutboxRelayMetrics(),
		"relay-test",
		log,
	)
	require.NoError(t, err)

	h := NewJobHandler(context.Background(), nil, nil, nil, nil, nil, relay, 10, testJobHandlerLogger())

	require.NoError(t, h.StartNotificationRelayJob(context.Background()))
	require.NoError(t, h.Stop())
}

type noopForwarder struct{}

func (noopForwarder) EnqueueNotification(context.Context, domain.NotificationOutboxRow, time.Time) error {
	return nil
}
