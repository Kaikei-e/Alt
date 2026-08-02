package datahub_gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"

	datahubv1 "alt/gen/proto/alt/datahub/v1"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
	"alt/orchestrator/driver/models"
)

type fakeRegisterFeedsClient struct {
	datahubv1connect.DataHubServiceClient
	calls  [][]*datahubv1.FeedRegistration
	failOn int
}

func (f *fakeRegisterFeedsClient) RegisterFeeds(_ context.Context, req *connect.Request[datahubv1.RegisterFeedsRequest]) (*connect.Response[datahubv1.RegisterFeedsResponse], error) {
	f.calls = append(f.calls, req.Msg.GetFeeds())
	if f.failOn > 0 && len(f.calls) == f.failOn {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("boom"))
	}
	results := make([]*datahubv1.FeedRegistrationResult, 0, len(req.Msg.GetFeeds()))
	for _, item := range req.Msg.GetFeeds() {
		results = append(results, &datahubv1.FeedRegistrationResult{FeedId: item.GetTitle(), Created: true})
	}
	return connect.NewResponse(&datahubv1.RegisterFeedsResponse{Results: results}), nil
}

func makeFeeds(n int) []models.Feed {
	now := time.Now().UTC()
	feeds := make([]models.Feed, n)
	for i := range feeds {
		feeds[i] = models.Feed{
			Title:      fmt.Sprintf("feed-%d", i),
			WebsiteURL: fmt.Sprintf("https://example.com/%d", i),
			PubDate:    now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}
	return feeds
}

func TestRegisterMultipleFeedsWithState_ChunksLargePolls(t *testing.T) {
	tests := []struct {
		name          string
		total         int
		wantCallSizes []int
	}{
		{name: "single call at the batch limit", total: 2000, wantCallSizes: []int{2000}},
		{name: "hourly poll above the limit is chunked", total: 2760, wantCallSizes: []int{2000, 760}},
		{name: "multiple full chunks", total: 4500, wantCallSizes: []int{2000, 2000, 500}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeRegisterFeedsClient{}
			g := NewFeedGateway(client)

			results, err := g.RegisterMultipleFeedsWithState(context.Background(), makeFeeds(tt.total))
			if err != nil {
				t.Fatalf("RegisterMultipleFeedsWithState: %v", err)
			}
			if len(results) != tt.total {
				t.Fatalf("results = %d, want %d", len(results), tt.total)
			}
			if len(client.calls) != len(tt.wantCallSizes) {
				t.Fatalf("RPC calls = %d, want %d", len(client.calls), len(tt.wantCallSizes))
			}
			for i, want := range tt.wantCallSizes {
				if got := len(client.calls[i]); got != want {
					t.Errorf("call %d size = %d, want %d", i, got, want)
				}
			}
			for i, r := range results {
				if want := fmt.Sprintf("feed-%d", i); r.FeedID != want {
					t.Fatalf("results[%d].FeedID = %q, want %q (order must survive chunking)", i, r.FeedID, want)
				}
			}
		})
	}
}

func TestRegisterMultipleFeedsWithState_ChunkErrorPropagates(t *testing.T) {
	client := &fakeRegisterFeedsClient{failOn: 2}
	g := NewFeedGateway(client)

	_, err := g.RegisterMultipleFeedsWithState(context.Background(), makeFeeds(2760))
	if err == nil {
		t.Fatal("want error from second chunk, got nil")
	}
	if len(client.calls) != 2 {
		t.Fatalf("RPC calls = %d, want 2 (stop at first failing chunk)", len(client.calls))
	}
}
