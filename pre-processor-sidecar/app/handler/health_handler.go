// ABOUTME: HealthHandler exposes /admin/health — the observable signal added after the
// ABOUTME: 2026-05-05 silent Inoreader outage so external watchers can detect token-source
// ABOUTME: loss or ingestion staleness without grepping logs.

package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// IngestionHealthProvider is the minimum surface HealthHandler reads to compute health.
// Implementations live in the service layer (InoreaderService + RemoteTokenService).
type IngestionHealthProvider interface {
	LastSuccessfulFetch() time.Time
	CircuitBreakerState() string
	TokenAvailable() bool
}

// HealthClock abstracts time.Now for deterministic tests.
type HealthClock interface {
	Now() time.Time
}

// HealthHandler serves /admin/health.
type HealthHandler struct {
	provider           IngestionHealthProvider
	clock              HealthClock
	silentThresholdSec int64
	logger             *slog.Logger
}

// NewHealthHandler constructs a HealthHandler. silentThresholdSeconds defines how stale
// LastSuccessfulFetch may be before the response flips to status="degraded" /
// ingestion_silent=true. Pass 1800 (30 min) to mirror the production runbook.
func NewHealthHandler(provider IngestionHealthProvider, clock HealthClock, silentThresholdSeconds int64, logger *slog.Logger) *HealthHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthHandler{
		provider:           provider,
		clock:              clock,
		silentThresholdSec: silentThresholdSeconds,
		logger:             logger,
	}
}

type healthPayload struct {
	Status                          string `json:"status"`
	TokenAvailable                  bool   `json:"token_available"`
	CircuitBreakerState             string `json:"circuit_breaker_state"`
	LastSuccessfulFetchAt           string `json:"last_successful_fetch_at"`
	SecondsSinceLastFetch           int64  `json:"seconds_since_last_fetch"`
	IngestionSilentThresholdSeconds int64  `json:"ingestion_silent_threshold_seconds"`
	IngestionSilent                 bool   `json:"ingestion_silent"`
}

// HandleHealth answers GET /admin/health with a JSON snapshot of ingestion health.
// 200 is returned even when status="degraded" — degraded is reported in the body so
// blackbox probes can observe the state machine without HTTP-status ambiguity.
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	now := h.clock.Now()
	last := h.provider.LastSuccessfulFetch()

	var lastStr string
	var sinceSec int64
	if last.IsZero() {
		lastStr = ""
		sinceSec = -1
	} else {
		lastStr = last.UTC().Format(time.RFC3339)
		sinceSec = int64(now.Sub(last).Seconds())
	}

	silent := sinceSec >= 0 && sinceSec >= h.silentThresholdSec
	if last.IsZero() {
		silent = true
	}

	tokenOk := h.provider.TokenAvailable()
	status := "ok"
	if silent || !tokenOk {
		status = "degraded"
	}

	payload := healthPayload{
		Status:                          status,
		TokenAvailable:                  tokenOk,
		CircuitBreakerState:             h.provider.CircuitBreakerState(),
		LastSuccessfulFetchAt:           lastStr,
		SecondsSinceLastFetch:           sinceSec,
		IngestionSilentThresholdSeconds: h.silentThresholdSec,
		IngestionSilent:                 silent,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.logger.Error("encode health payload", "error", err)
	}
}
