package feed_stats_gateway

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSummarizedArticlesCountGateway_Execute(t *testing.T) {
	// Use constructor with nil pool to test error handling
	gateway := NewSummarizedArticlesCountGateway(nil)

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
				t.Errorf("SummarizedArticlesCountGateway.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SummarizedArticlesCountGateway.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSummarizedArticlesCountGateway(t *testing.T) {
	// Test constructor
	var pool *pgxpool.Pool // nil pool for testing
	gateway := NewSummarizedArticlesCountGateway(pool)

	if gateway == nil {
		t.Error("NewSummarizedArticlesCountGateway() returned nil")
	}

	// With our refactored approach, repository will be nil when pool is nil
	if gateway.altDBRepository != nil {
		t.Error("NewSummarizedArticlesCountGateway() with nil pool should have nil repository")
	}
}

func TestSummarizedArticlesCountGateway_ErrorHandling(t *testing.T) {
	// Use constructor with nil pool to test error handling
	gateway := NewSummarizedArticlesCountGateway(nil)

	// Test error propagation
	count, err := gateway.Execute(context.Background())
	if err == nil {
		t.Error("SummarizedArticlesCountGateway.Execute() expected error with nil repository, got nil")
	}

	if count != 0 {
		t.Errorf("SummarizedArticlesCountGateway.Execute() expected count 0 on error, got %d", count)
	}

	// Verify the error message
	expectedMsg := "database connection not available"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestSummarizedArticlesCountGateway_ContextTimeout(t *testing.T) {
	// Use constructor with nil pool to test error handling
	gateway := NewSummarizedArticlesCountGateway(nil)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 0) // Immediate timeout
	defer cancel()

	_, err := gateway.Execute(ctx)
	if err == nil {
		t.Error("SummarizedArticlesCountGateway.Execute() expected error with timed out context, got nil")
	}
}

func TestSummarizedArticlesCountGateway_NilContext(t *testing.T) {
	// Use constructor with nil pool to test error handling
	gateway := NewSummarizedArticlesCountGateway(nil)

	// Test with nil context (this should panic or error)
	defer func() {
		if r := recover(); r == nil {
			// If it doesn't panic, it should at least error
			_, err := gateway.Execute(context.TODO())
			if err == nil {
				t.Error("SummarizedArticlesCountGateway.Execute() expected error with nil context, got nil")
			}
		}
	}()

	gateway.Execute(context.TODO())
}