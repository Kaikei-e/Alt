package feed_stats_gateway

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFeedAmountGateway_Execute(t *testing.T) {
	gateway := &FeedAmountGateway{
		altDBRepository: nil, // This will cause an error, which we can test
	}

	type args struct {
		ctx context.Context
	}

	tests := []struct {
		name    string
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "execute with nil database (should error)",
			args: args{
				ctx: context.Background(),
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "execute with cancelled context",
			args: args{
				ctx: func() context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx
				}(),
			},
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gateway.Execute(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("FeedAmountGateway.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FeedAmountGateway.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewFeedAmountGateway(t *testing.T) {
	// Test constructor
	var pool *pgxpool.Pool // nil pool for testing
	gateway := NewFeedAmountGateway(pool)
	
	if gateway == nil {
		t.Error("NewFeedAmountGateway() returned nil")
	}
	
	if gateway.altDBRepository == nil {
		t.Error("NewFeedAmountGateway() altDBRepository should be initialized")
	}
}

func TestFeedAmountGateway_ErrorHandling(t *testing.T) {
	gateway := &FeedAmountGateway{
		altDBRepository: nil,
	}

	// Test error propagation
	count, err := gateway.Execute(context.Background())
	if err == nil {
		t.Error("FeedAmountGateway.Execute() expected error with nil repository, got nil")
	}
	
	if count != 0 {
		t.Errorf("FeedAmountGateway.Execute() expected count 0 on error, got %d", count)
	}
}

func TestFeedAmountGateway_ContextTimeout(t *testing.T) {
	gateway := &FeedAmountGateway{
		altDBRepository: nil,
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 0) // Immediate timeout
	defer cancel()

	_, err := gateway.Execute(ctx)
	if err == nil {
		t.Error("FeedAmountGateway.Execute() expected error with timed out context, got nil")
	}
}

func TestFeedAmountGateway_NilContext(t *testing.T) {
	gateway := &FeedAmountGateway{
		altDBRepository: nil,
	}

	// Test with nil context (this should panic or error)
	defer func() {
		if r := recover(); r == nil {
			// If it doesn't panic, it should at least error
			_, err := gateway.Execute(nil)
			if err == nil {
				t.Error("FeedAmountGateway.Execute() expected error with nil context, got nil")
			}
		}
	}()

	gateway.Execute(nil)
}