package di

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"alt/config"
	"alt/orchestrator/connect/v2/articles"
)

// The composition root must declare the prefetch capability, not leave it to
// be inferred from whether a pointer happens to be nil (ADR-000966 §2). Every
// port behind a prefetch is shared with the interactive read path, so their
// presence says nothing at all about whether warming is meant to happen.
func TestNewArticlePrefetchWiring(t *testing.T) {
	t.Run("a duration enables it with that budget", func(t *testing.T) {
		wiring, err := newArticlePrefetchWiring(config.RateLimitConfig{PrefetchSlotWait: "250ms"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !wiring.Enabled {
			t.Fatal("expected prefetch to be enabled")
		}
		if wiring.SlotWait != 250*time.Millisecond {
			t.Fatalf("SlotWait = %v, want 250ms", wiring.SlotWait)
		}
	})

	t.Run("off disables it and names the setting", func(t *testing.T) {
		wiring, err := newArticlePrefetchWiring(config.RateLimitConfig{PrefetchSlotWait: "off"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if wiring.Enabled {
			t.Fatal("expected prefetch to be disabled")
		}
		if !strings.Contains(wiring.DisabledReason, "RATE_LIMIT_PREFETCH_SLOT_WAIT") {
			t.Fatalf("DisabledReason must name the setting to change, got %q", wiring.DisabledReason)
		}
	})

	t.Run("an unusable value is an error, never a guess", func(t *testing.T) {
		if _, err := newArticlePrefetchWiring(config.RateLimitConfig{PrefetchSlotWait: "0s"}); err == nil {
			t.Fatal("expected a zero budget to be refused")
		}
	})
}

// Rule 8: whichever state the process is in, it says so once at startup, by
// name. Silence would make "prefetch is off" and "somebody forgot to wire
// prefetch" the same observation from the log.
func TestLogArticlePrefetchWiringDeclaresBothStates(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		line := captureSlogLine(t, func() {
			logArticlePrefetchWiring(articles.ArticlePrefetchWiring{Enabled: true, SlotWait: 250 * time.Millisecond})
		})
		if line["msg"] != "article_prefetch.enabled" {
			t.Fatalf("msg = %v, want article_prefetch.enabled", line["msg"])
		}
		if line["slot_wait"] == nil {
			t.Fatal("the enabled line must state the budget it was wired with")
		}
		if line["namespace"] != "external_api" {
			t.Fatalf("the enabled line must name the shared rate-limit namespace, got %v", line["namespace"])
		}
	})

	t.Run("disabled", func(t *testing.T) {
		line := captureSlogLine(t, func() {
			logArticlePrefetchWiring(articles.ArticlePrefetchWiring{
				Enabled:        false,
				DisabledReason: "RATE_LIMIT_PREFETCH_SLOT_WAIT=off",
			})
		})
		if line["msg"] != "article_prefetch.disabled" {
			t.Fatalf("msg = %v, want article_prefetch.disabled", line["msg"])
		}
		if line["level"] != "WARN" {
			t.Fatalf("a missing capability is a WARN, got %v", line["level"])
		}
		if line["reason"] == nil || line["impact"] == nil {
			t.Fatalf("the disabled line must state why and what it costs, got %v", line)
		}
	})
}

func captureSlogLine(t *testing.T, emit func()) map[string]any {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	emit()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one declaration, got %d: %s", len(lines), buf.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, lines[0])
	}
	return parsed
}
