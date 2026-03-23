package sovereign_client

import (
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
)

// ProjectorListener implements the KnowledgeProjectorListener interface
// using a Connect-RPC server-streaming RPC from sovereign.
//
// A single pump goroutine reads from the stream and sends to a channel.
// WaitForNotification selects on that channel + caller context, avoiding
// goroutine leaks and composing correctly with the runner's poll-timeout model.
type ProjectorListener struct {
	cancel context.CancelFunc
	notify chan struct{}
	errCh  chan error
}

// ConnectProjectorWatch creates a new streaming connection to WatchProjectorEvents
// and starts a background pump goroutine.
func (c *Client) ConnectProjectorWatch(ctx context.Context, projectorName string) (*ProjectorListener, error) {
	if !c.enabled {
		return nil, fmt.Errorf("sovereign client not enabled")
	}

	// Streaming needs a separate HTTP client without the 30s global timeout.
	// The main c.client uses Timeout:30s which kills long-lived streams.
	streamHTTPClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        5,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
		},
		// No Timeout — streaming connections are long-lived.
	}
	streamClient := sovereignv1connect.NewKnowledgeSovereignServiceClient(
		streamHTTPClient,
		c.baseURL,
	)

	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := streamClient.WatchProjectorEvents(streamCtx, connect.NewRequest(&sovereignv1.WatchProjectorEventsRequest{
		ProjectorName: projectorName,
	}))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("sovereign WatchProjectorEvents: %w", err)
	}

	l := &ProjectorListener{
		cancel: cancel,
		notify: make(chan struct{}, 1),
		errCh:  make(chan error, 1),
	}

	// Single pump goroutine — runs for the lifetime of the listener.
	// Reads stream messages and signals the notify channel.
	go func() {
		defer close(l.notify)
		for stream.Receive() {
			select {
			case l.notify <- struct{}{}:
			default: // drop if runner hasn't consumed yet
			}
		}
		if err := stream.Err(); err != nil {
			select {
			case l.errCh <- fmt.Errorf("sovereign watch stream: %w", err):
			default:
			}
		}
	}()

	slog.Info("sovereign projector listener connected", "projector_name", projectorName)
	return l, nil
}

// WaitForNotification blocks until a notification arrives, the stream errors,
// or the caller's context expires.
func (l *ProjectorListener) WaitForNotification(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case _, ok := <-l.notify:
		if !ok {
			// Channel closed — stream ended. Check for error.
			select {
			case err := <-l.errCh:
				return err
			default:
				return fmt.Errorf("sovereign watch stream closed")
			}
		}
		return nil
	case err := <-l.errCh:
		return err
	}
}

// Close terminates the streaming connection and the pump goroutine.
func (l *ProjectorListener) Close(_ context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	return nil
}
