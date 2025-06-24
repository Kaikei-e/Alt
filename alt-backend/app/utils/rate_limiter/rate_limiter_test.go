package rate_limiter

import (
	"context"
	"testing"
	"time"
)

func TestNewHostRateLimiter(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{
			name:     "creates rate limiter with 5 second interval",
			interval: 5 * time.Second,
			want:     5 * time.Second,
		},
		{
			name:     "creates rate limiter with 1 second interval",
			interval: 1 * time.Second,
			want:     1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewHostRateLimiter(tt.interval)
			if limiter == nil {
				t.Error("NewHostRateLimiter() returned nil")
				return
			}
			if limiter.interval != tt.want {
				t.Errorf("NewHostRateLimiter() interval = %v, want %v", limiter.interval, tt.want)
			}
			if limiter.limiters == nil {
				t.Error("NewHostRateLimiter() limiters map is nil")
			}
		})
	}
}

func TestHostRateLimiter_WaitForHost(t *testing.T) {
	tests := []struct {
		name    string
		urlStr  string
		wantErr bool
	}{
		{
			name:    "valid http URL",
			urlStr:  "http://example.com/feed.xml",
			wantErr: false,
		},
		{
			name:    "valid https URL",
			urlStr:  "https://example.com/feed.xml",
			wantErr: false,
		},
		{
			name:    "URL with path",
			urlStr:  "https://feeds.feedburner.com/example/feed",
			wantErr: false,
		},
		{
			name:    "invalid URL",
			urlStr:  "not-a-url",
			wantErr: true,
		},
		{
			name:    "empty URL",
			urlStr:  "",
			wantErr: true,
		},
	}

	limiter := NewHostRateLimiter(100 * time.Millisecond) // Fast interval for testing

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := limiter.WaitForHost(ctx, tt.urlStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("WaitForHost() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHostRateLimiter_RateLimitingBehavior(t *testing.T) {
	limiter := NewHostRateLimiter(200 * time.Millisecond)
	ctx := context.Background()
	
	url1 := "https://example.com/feed1"
	url2 := "https://example.com/feed2"
	url3 := "https://different.com/feed"

	// First call should be immediate
	start := time.Now()
	err := limiter.WaitForHost(ctx, url1)
	if err != nil {
		t.Fatalf("First WaitForHost() failed: %v", err)
	}
	firstCallDuration := time.Since(start)
	
	// Should be nearly immediate (less than 50ms)
	if firstCallDuration > 50*time.Millisecond {
		t.Errorf("First call took too long: %v", firstCallDuration)
	}

	// Second call to same host should be rate limited
	start = time.Now()
	err = limiter.WaitForHost(ctx, url2) // Same host: example.com
	if err != nil {
		t.Fatalf("Second WaitForHost() failed: %v", err)
	}
	secondCallDuration := time.Since(start)
	
	// Should wait for rate limit (approximately 200ms)
	if secondCallDuration < 150*time.Millisecond {
		t.Errorf("Second call was not rate limited: %v", secondCallDuration)
	}

	// Call to different host should be immediate
	start = time.Now()
	err = limiter.WaitForHost(ctx, url3) // Different host: different.com
	if err != nil {
		t.Fatalf("Third WaitForHost() failed: %v", err)
	}
	thirdCallDuration := time.Since(start)
	
	// Should be immediate for different host
	if thirdCallDuration > 50*time.Millisecond {
		t.Errorf("Third call (different host) took too long: %v", thirdCallDuration)
	}
}

func TestHostRateLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewHostRateLimiter(100 * time.Millisecond)
	ctx := context.Background()
	
	url := "https://example.com/feed"
	
	// Test concurrent access to same host
	done := make(chan bool, 2)
	
	go func() {
		err := limiter.WaitForHost(ctx, url)
		if err != nil {
			t.Errorf("Concurrent call 1 failed: %v", err)
		}
		done <- true
	}()
	
	go func() {
		err := limiter.WaitForHost(ctx, url)
		if err != nil {
			t.Errorf("Concurrent call 2 failed: %v", err)
		}
		done <- true
	}()
	
	// Wait for both goroutines to complete
	<-done
	<-done
}

func TestHostRateLimiter_ContextCancellation(t *testing.T) {
	limiter := NewHostRateLimiter(1 * time.Second) // Long interval
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Start a goroutine that will cancel the context after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	
	url := "https://example.com/feed"
	
	// First call to establish rate limiter state
	err := limiter.WaitForHost(context.Background(), url)
	if err != nil {
		t.Fatalf("Setup call failed: %v", err)
	}
	
	// Second call should be cancelled
	start := time.Now()
	err = limiter.WaitForHost(ctx, url)
	duration := time.Since(start)
	
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
	
	// Should be cancelled within reasonable time
	if duration > 200*time.Millisecond {
		t.Errorf("Context cancellation took too long: %v", duration)
	}
}