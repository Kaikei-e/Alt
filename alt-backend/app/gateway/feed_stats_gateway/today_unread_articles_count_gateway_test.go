package feed_stats_gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTodayUnreadArticlesCountGateway_Execute(t *testing.T) {
	gateway := NewTodayUnreadArticlesCountGateway(nil)

	type args struct{ ctx context.Context }
	tests := []struct {
		name    string
		args    args
		want    int
		wantErr bool
	}{
		{
			name:    "execute with nil database (should error)",
			args:    args{ctx: context.Background()},
			want:    0,
			wantErr: true,
		},
		{
			name:    "execute with cancelled context",
			args:    args{ctx: func() context.Context { c, cancel := context.WithCancel(context.Background()); cancel(); return c }()},
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gateway.Execute(tt.args.ctx, time.Now())
			if (err != nil) != tt.wantErr {
				t.Errorf("TodayUnreadArticlesCountGateway.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TodayUnreadArticlesCountGateway.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewTodayUnreadArticlesCountGateway(t *testing.T) {
	var pool *pgxpool.Pool
	gateway := NewTodayUnreadArticlesCountGateway(pool)
	if gateway == nil {
		t.Error("NewTodayUnreadArticlesCountGateway() returned nil")
	}
	if gateway.altDBRepository != nil {
		t.Error("NewTodayUnreadArticlesCountGateway() with nil pool should have nil repository")
	}
}

func TestTodayUnreadArticlesCountGateway_ErrorHandling(t *testing.T) {
	gateway := NewTodayUnreadArticlesCountGateway(nil)
	count, err := gateway.Execute(context.Background(), time.Now())
	if err == nil {
		t.Error("TodayUnreadArticlesCountGateway.Execute() expected error with nil repository, got nil")
	}
	if count != 0 {
		t.Errorf("TodayUnreadArticlesCountGateway.Execute() expected count 0 on error, got %d", count)
	}
	if err == nil || !strings.Contains(err.Error(), "database connection not available") {
		t.Errorf("Expected error message to contain 'database connection not available', got '%v'", err)
	}
}

func TestTodayUnreadArticlesCountGateway_ContextTimeout(t *testing.T) {
	gateway := NewTodayUnreadArticlesCountGateway(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	_, err := gateway.Execute(ctx, time.Now())
	if err == nil {
		t.Error("TodayUnreadArticlesCountGateway.Execute() expected error with timed out context, got nil")
	}
}

func TestTodayUnreadArticlesCountGateway_NilContext(t *testing.T) {
	gateway := NewTodayUnreadArticlesCountGateway(nil)
	defer func() {
		if r := recover(); r == nil {
			_, err := gateway.Execute(context.TODO(), time.Now())
			if err == nil {
				t.Error("TodayUnreadArticlesCountGateway.Execute() expected error with nil context, got nil")
			}
		}
	}()
	gateway.Execute(context.TODO(), time.Now())
}
