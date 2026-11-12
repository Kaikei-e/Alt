package altdb

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestDataHubTagCloudClient_RejectsEmptyUserID(t *testing.T) {
	// A nil RPC client proves the guard fires before any request is built.
	client := NewDataHubTagCloudClient(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	tags, err := client.FetchTagCloud(context.Background(), "", 300)
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
	if tags != nil {
		t.Errorf("expected no tags, got %v", tags)
	}
}
