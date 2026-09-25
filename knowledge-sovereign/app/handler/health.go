package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// HealthHandler returns the service health status.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "knowledge-sovereign"}); err != nil {
		slog.WarnContext(r.Context(), "failed to write health response", "error", err)
	}
}
