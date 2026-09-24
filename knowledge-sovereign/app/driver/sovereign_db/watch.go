package sovereign_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// WatchChannel is the PostgreSQL LISTEN channel for knowledge projector notifications.
	WatchChannel = "knowledge_projector"
	// WatchHeartbeatInterval is the duration between heartbeat messages when no notifications arrive.
	WatchHeartbeatInterval = 3 * time.Second
)

// ProjectorEventWatcher manages a dedicated PostgreSQL connection listening for notifications.
type ProjectorEventWatcher struct {
	conn *pgx.Conn
}

// OpenProjectorEventWatcher connects to databaseURL and begins LISTEN on the watch channel.
func OpenProjectorEventWatcher(ctx context.Context, databaseURL string) (*ProjectorEventWatcher, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect for LISTEN: %w", err)
	}

	if _, err := conn.Exec(ctx, "LISTEN "+WatchChannel); err != nil {
		_ = conn.Close(context.Background())
		return nil, fmt.Errorf("LISTEN %s: %w", WatchChannel, err)
	}

	return &ProjectorEventWatcher{conn: conn}, nil
}

// Close closes the underlying connection.
func (w *ProjectorEventWatcher) Close(ctx context.Context) error {
	if w.conn != nil {
		return w.conn.Close(ctx)
	}
	return nil
}

// WaitForNotification waits for a notification on the watched channel until the timeout expires.
// Returns (payload, isTimeout, err). On timeout, isTimeout is true and err is nil.
func (w *ProjectorEventWatcher) WaitForNotification(ctx context.Context, timeout time.Duration) (string, bool, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	notification, err := w.conn.WaitForNotification(waitCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", true, nil
		}
		return "", false, err
	}

	return notification.Payload, false, nil
}
