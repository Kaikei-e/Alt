package proxy

import (
	"testing"

	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/config"
)

// TestNewLightweightProxy_AutoLearnerIsProperlyWired guards against the
// regression where NewLightweightProxy built autoLearner as a bare
// &autolearn.AutoLearner{} (bypassing NewAutoLearner), leaving its
// domains/config/logger/validator/rateLimiter fields nil. That zero value
// panics with a nil pointer dereference the moment LearnDomain is reached.
func TestNewLightweightProxy_AutoLearnerIsProperlyWired(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("config.LoadConfig() error = %v", err)
	}

	p, err := NewLightweightProxy(cfg)
	if err != nil {
		t.Fatalf("NewLightweightProxy() error = %v", err)
	}

	// A zero-value AutoLearner panics here (nil *autolearn.Config
	// dereference in LearnDomain). A properly constructed one returns a
	// normal error since learning is disabled by design.
	if err := p.autoLearner.LearnDomain("sub.example.com", "http://sub.example.com/feed", "trace-1"); err == nil {
		t.Fatal("LearnDomain() error = nil, want the 'auto-learning is disabled' error")
	}
}
