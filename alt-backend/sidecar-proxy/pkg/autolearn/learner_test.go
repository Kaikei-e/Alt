package autolearn

import (
	"io"
	"log"
	"testing"
)

func testLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// TestNewAutoLearner_LearnDomainDoesNotPanicWhenDisabled guards against the
// regression where the proxy constructed a bare &AutoLearner{} (bypassing
// NewAutoLearner), leaving config/validator/rateLimiter/logger nil. Calling
// LearnDomain on that zero value panics on the nil *Config dereference
// (al.config.LearningEnabled). A properly constructed AutoLearner must
// instead return a normal "disabled" error.
func TestNewAutoLearner_LearnDomainDoesNotPanicWhenDisabled(t *testing.T) {
	al, err := NewAutoLearner(&Config{
		MaxDomains:       10,
		LearningEnabled:  false,
		SecurityLevel:    "strict",
		RateLimitPerHour: 5,
		CooldownMinutes:  1,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewAutoLearner() error = %v", err)
	}

	err = al.LearnDomain("news.example.org", "http://news.example.org/feed", "trace-1")
	if err == nil {
		t.Fatal("LearnDomain() error = nil, want an 'auto-learning is disabled' error")
	}

	if al.IsLearningEnabled() {
		t.Error("IsLearningEnabled() = true, want false")
	}
}

func TestNewAutoLearner_LearnDomainSucceedsWhenEnabled(t *testing.T) {
	al, err := NewAutoLearner(&Config{
		MaxDomains:       10,
		LearningEnabled:  true,
		SecurityLevel:    "strict",
		RateLimitPerHour: 5,
		CooldownMinutes:  1,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewAutoLearner() error = %v", err)
	}

	if !al.IsLearningEnabled() {
		t.Error("IsLearningEnabled() = false, want true")
	}

	const domain = "readernews.io"
	if err := al.LearnDomain(domain, "http://"+domain+"/feed", "trace-1"); err != nil {
		t.Fatalf("LearnDomain() error = %v, want nil", err)
	}

	if !al.IsAllowed(domain) {
		t.Error("IsAllowed() = false after successful LearnDomain, want true")
	}
}

func TestNewAutoLearner_ZeroValueWouldPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a zero-value &AutoLearner{} to panic on LearnDomain (nil *Config); it did not")
		}
	}()

	al := &AutoLearner{}
	_ = al.LearnDomain("example.com", "http://example.com", "trace-1")
}
