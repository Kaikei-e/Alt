package bootstrap

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"pre-processor/metrics"
)

// TestBuildNotificationRelay_RequiresTheDataHubClient keeps the relay from
// being constructed around a missing peer and then reporting healthy ticks
// that forward nothing.
func TestBuildNotificationRelay_RequiresTheDataHubClient(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	_, err := buildNotificationRelay(nil, nil, metrics.NewOutboxRelayMetrics(), log)

	require.Error(t, err)
}
